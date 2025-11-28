package plugin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/smykla-labs/klaudiush/api/plugin/v1"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

const (
	// defaultGRPCTimeout is the default timeout for gRPC operations.
	defaultGRPCTimeout = 5 * time.Second

	// defaultDialTimeout is the default timeout for establishing gRPC connections.
	defaultDialTimeout = 10 * time.Second
)

var (
	// ErrGRPCAddressRequired is returned when address is missing for gRPC plugins.
	ErrGRPCAddressRequired = errors.New("address is required for gRPC plugins")

	// ErrGRPCInfoFailed is returned when fetching plugin info fails.
	ErrGRPCInfoFailed = errors.New("failed to fetch plugin info via gRPC")

	// ErrGRPCNilResponse is returned when gRPC returns a nil response.
	ErrGRPCNilResponse = errors.New("gRPC returned nil response")

	// ErrTLSCertLoad is returned when TLS certificate loading fails.
	ErrTLSCertLoad = errors.New("failed to load TLS certificate")

	// ErrTLSCALoad is returned when CA certificate loading fails.
	ErrTLSCALoad = errors.New("failed to load CA certificate")
)

// GRPCLoader loads plugins via gRPC and maintains a connection pool.
//
// Connections are pooled by address and reused across multiple plugin instances
// to reduce overhead. All connections are closed when the loader is closed.
type GRPCLoader struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn
	dialTimeout time.Duration
	closed      bool
	logger      logger.Logger
}

// NewGRPCLoader creates a new gRPC plugin loader with connection pooling.
func NewGRPCLoader() *GRPCLoader {
	return &GRPCLoader{
		connections: make(map[string]*grpc.ClientConn),
		dialTimeout: defaultDialTimeout,
		logger:      logger.Default(),
	}
}

// NewGRPCLoaderWithTimeout creates a new gRPC plugin loader with a custom dial timeout.
func NewGRPCLoaderWithTimeout(dialTimeout time.Duration) *GRPCLoader {
	return &GRPCLoader{
		connections: make(map[string]*grpc.ClientConn),
		dialTimeout: dialTimeout,
		logger:      logger.Default(),
	}
}

// NewGRPCLoaderWithLogger creates a new gRPC plugin loader with a custom logger.
func NewGRPCLoaderWithLogger(log logger.Logger) *GRPCLoader {
	return &GRPCLoader{
		connections: make(map[string]*grpc.ClientConn),
		dialTimeout: defaultDialTimeout,
		logger:      log,
	}
}

// Load loads a gRPC plugin from the specified address.
//
// The dial timeout from the loader is used for initial connection establishment.
// The timeout from the config (or defaultGRPCTimeout) is used for subsequent RPC calls.
//
//nolint:ireturn // interface return is required by Loader interface
func (l *GRPCLoader) Load(cfg *config.PluginInstanceConfig) (Plugin, error) {
	// Check if loader has been closed
	l.mu.RLock()

	if l.closed {
		l.mu.RUnlock()

		return nil, ErrLoaderClosed
	}

	l.mu.RUnlock()

	if cfg.Address == "" {
		return nil, ErrGRPCAddressRequired
	}

	// Build transport credentials based on TLS config
	creds, err := l.buildTransportCredentials(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build transport credentials")
	}

	// Create context with dial timeout for connection establishment
	ctx, cancel := context.WithTimeout(context.Background(), l.dialTimeout)
	defer cancel()

	// Get or create connection
	conn, err := l.getOrCreateConnection(ctx, cfg.Address, creds)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish gRPC connection")
	}

	// Create client
	client := pluginv1.NewValidatorPluginClient(conn)

	// Fetch plugin info (reuses ctx if not expired, otherwise creates new)
	info, err := l.fetchInfo(ctx, client, cfg.GetTimeout(defaultGRPCTimeout))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch plugin info")
	}

	return &grpcPluginAdapter{
		client:  client,
		timeout: cfg.GetTimeout(defaultGRPCTimeout),
		config:  cfg.Config,
		info:    info,
	}, nil
}

// Close releases all gRPC connections held by the loader.
// After Close is called, Load will return ErrLoaderClosed.
func (l *GRPCLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true

	var errs []error

	for addr, conn := range l.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to close connection to %s", addr))
		}
	}

	l.connections = make(map[string]*grpc.ClientConn)

	if len(errs) > 0 {
		return errors.Errorf("failed to close %d connections: %v", len(errs), errs)
	}

	return nil
}

// getOrCreateConnection gets an existing connection or creates a new one.
//
// Note: grpc.NewClient is lazy and doesn't establish connection immediately.
// The dialTimeout will be enforced when the first RPC is made (e.g., Info call).
func (l *GRPCLoader) getOrCreateConnection(
	_ context.Context,
	address string,
	creds credentials.TransportCredentials,
) (*grpc.ClientConn, error) {
	// Fast path: check if connection exists
	l.mu.RLock()
	existingConn, exists := l.connections[address]
	l.mu.RUnlock()

	if exists {
		return existingConn, nil
	}

	// Slow path: create new connection
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check closed flag after acquiring lock to prevent race with Close()
	if l.closed {
		return nil, ErrLoaderClosed
	}

	// Double-check after acquiring write lock
	if existingConn, exists := l.connections[address]; exists {
		return existingConn, nil
	}

	// Create new connection (lazy, won't dial until first RPC)
	// The parent context with dialTimeout will be used for the first RPC
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create gRPC client for %s", address)
	}

	l.connections[address] = conn

	return conn, nil
}

// fetchInfo fetches plugin metadata via gRPC.
func (*GRPCLoader) fetchInfo(
	parentCtx context.Context,
	client pluginv1.ValidatorPluginClient,
	timeout time.Duration,
) (plugin.Info, error) {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	resp, err := client.Info(ctx, &pluginv1.InfoRequest{})
	if err != nil {
		return plugin.Info{}, errors.Wrapf(ErrGRPCInfoFailed, "gRPC error: %v", err)
	}

	// Defense-in-depth nil check (protobuf getters handle nil, but explicit check is safer)
	if resp == nil {
		return plugin.Info{}, ErrGRPCNilResponse
	}

	return plugin.Info{
		Name:        resp.GetName(),
		Version:     resp.GetVersion(),
		Description: resp.GetDescription(),
		Author:      resp.GetAuthor(),
		URL:         resp.GetUrl(),
	}, nil
}

// buildTransportCredentials builds appropriate transport credentials based on config.
//
// TLS behavior:
//   - nil TLS config + localhost: insecure (backward compatible)
//   - nil TLS config + remote: error (require explicit config)
//   - TLS enabled: use TLS with optional client certs
//   - TLS disabled + localhost: insecure
//   - TLS disabled + remote: error unless AllowInsecureRemote is set
//
//nolint:ireturn // interface return is required by gRPC credentials
func (l *GRPCLoader) buildTransportCredentials(
	cfg *config.PluginInstanceConfig,
) (credentials.TransportCredentials, error) {
	isLocal := IsLocalAddress(cfg.Address)
	tlsCfg := cfg.TLS

	// Auto mode (nil config or nil Enabled): insecure for localhost, error for remote
	if tlsCfg == nil || tlsCfg.IsEnabled() == nil {
		if isLocal {
			return insecure.NewCredentials(), nil
		}

		return nil, errors.Wrapf(
			ErrInsecureRemote,
			"TLS required for remote address %s; set tls.enabled=true or tls.allow_insecure_remote=true",
			cfg.Address,
		)
	}

	// Explicit insecure to remote - check if allowed
	if !*tlsCfg.Enabled && !isLocal {
		if tlsCfg.AllowsInsecureRemote() {
			l.logger.Info("WARNING: insecure connection to remote gRPC plugin",
				"address", cfg.Address,
				"plugin", cfg.Name)

			return insecure.NewCredentials(), nil
		}

		return nil, errors.Wrapf(
			ErrInsecureRemote,
			"insecure connection to remote address %s not allowed; set tls.allow_insecure_remote=true to override",
			cfg.Address,
		)
	}

	// Explicit insecure to localhost
	if !*tlsCfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	// Build TLS credentials
	return l.buildTLSCredentials(tlsCfg)
}

// buildTLSCredentials builds TLS transport credentials from the config.
//
//nolint:ireturn // interface return is required by gRPC credentials
func (*GRPCLoader) buildTLSCredentials(
	cfg *config.TLSConfig,
) (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load CA certificate if specified
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, errors.Wrapf(ErrTLSCALoad, "failed to read CA file %s: %v", cfg.CAFile, err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caCert) {
			return nil, errors.Wrapf(
				ErrTLSCALoad,
				"failed to parse CA certificate from %s",
				cfg.CAFile,
			)
		}

		tlsConfig.RootCAs = certPool
	}

	// Validate mTLS configuration - both files must be specified together
	hasCertFile := cfg.CertFile != ""
	hasKeyFile := cfg.KeyFile != ""

	if hasCertFile != hasKeyFile {
		return nil, errors.Wrap(ErrTLSCertLoad,
			"both cert_file and key_file must be specified for client certificate authentication")
	}

	// Load client certificate if specified (mTLS)
	if hasCertFile && hasKeyFile {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, errors.Wrapf(ErrTLSCertLoad,
				"failed to load client certificate from %s and %s: %v",
				cfg.CertFile, cfg.KeyFile, err)
		}

		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Handle InsecureSkipVerify
	if cfg.ShouldSkipVerify() {
		tlsConfig.InsecureSkipVerify = true
	}

	return credentials.NewTLS(tlsConfig), nil
}

// grpcPluginAdapter adapts a gRPC plugin to the internal Plugin interface.
type grpcPluginAdapter struct {
	client  pluginv1.ValidatorPluginClient
	timeout time.Duration
	config  map[string]any
	info    plugin.Info
}

// Info returns metadata about the plugin.
func (a *grpcPluginAdapter) Info() plugin.Info {
	return a.info
}

// Validate performs validation via gRPC.
func (a *grpcPluginAdapter) Validate(
	ctx context.Context,
	req *plugin.ValidateRequest,
) (*plugin.ValidateResponse, error) {
	// Apply timeout if context doesn't have one
	execCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc

		execCtx, cancel = context.WithTimeout(ctx, a.timeout)

		defer cancel()
	}

	// Convert internal request to protobuf request
	protoReq, err := a.toProtoRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert request to protobuf")
	}

	// Make gRPC call
	protoResp, err := a.client.Validate(execCtx, protoReq)
	if err != nil {
		return nil, errors.Wrap(err, "gRPC validate call failed")
	}

	// Convert protobuf response to internal response
	return a.fromProtoResponse(protoResp), nil
}

// Close releases any resources held by the plugin.
func (*grpcPluginAdapter) Close() error {
	// Connection is managed by the loader, not the individual adapter
	return nil
}

// toProtoRequest converts internal ValidateRequest to protobuf ValidateRequest.
func (a *grpcPluginAdapter) toProtoRequest(
	req *plugin.ValidateRequest,
) (*pluginv1.ValidateRequest, error) {
	// Add plugin-specific config to the request
	cfg := req.Config
	if cfg == nil && len(a.config) > 0 {
		cfg = a.config
	}

	// Convert map[string]any to map[string]string for protobuf
	configMap := make(map[string]string, len(cfg))
	for k, v := range cfg {
		// Convert value to JSON string for non-string types
		switch val := v.(type) {
		case string:
			configMap[k] = val
		default:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal config value for key %s", k)
			}

			configMap[k] = string(jsonBytes)
		}
	}

	return &pluginv1.ValidateRequest{
		EventType: req.EventType,
		ToolName:  req.ToolName,
		Command:   req.Command,
		FilePath:  req.FilePath,
		Content:   req.Content,
		OldString: req.OldString,
		NewString: req.NewString,
		Pattern:   req.Pattern,
		Config:    configMap,
	}, nil
}

// fromProtoResponse converts protobuf ValidateResponse to internal ValidateResponse.
func (*grpcPluginAdapter) fromProtoResponse(
	resp *pluginv1.ValidateResponse,
) *plugin.ValidateResponse {
	return &plugin.ValidateResponse{
		Passed:      resp.GetPassed(),
		ShouldBlock: resp.GetShouldBlock(),
		Message:     resp.GetMessage(),
		ErrorCode:   resp.GetErrorCode(),
		FixHint:     resp.GetFixHint(),
		DocLink:     resp.GetDocLink(),
		Details:     resp.GetDetails(),
	}
}
