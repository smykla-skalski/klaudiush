package plugin

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/smykla-labs/klaudiush/api/plugin/v1"
	"github.com/smykla-labs/klaudiush/pkg/config"
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
)

// GRPCLoader loads plugins via gRPC and maintains a connection pool.
//
// Connections are pooled by address and reused across multiple plugin instances
// to reduce overhead. All connections are closed when the loader is closed.
type GRPCLoader struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn
	dialTimeout time.Duration
}

// NewGRPCLoader creates a new gRPC plugin loader with connection pooling.
func NewGRPCLoader() *GRPCLoader {
	return &GRPCLoader{
		connections: make(map[string]*grpc.ClientConn),
		dialTimeout: defaultDialTimeout,
	}
}

// NewGRPCLoaderWithTimeout creates a new gRPC plugin loader with a custom dial timeout.
func NewGRPCLoaderWithTimeout(dialTimeout time.Duration) *GRPCLoader {
	return &GRPCLoader{
		connections: make(map[string]*grpc.ClientConn),
		dialTimeout: dialTimeout,
	}
}

// Load loads a gRPC plugin from the specified address.
//
//nolint:ireturn // interface return is required by Loader interface
func (l *GRPCLoader) Load(cfg *config.PluginInstanceConfig) (Plugin, error) {
	if cfg.Address == "" {
		return nil, ErrGRPCAddressRequired
	}

	// Get or create connection
	conn, err := l.getOrCreateConnection(cfg.Address)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish gRPC connection")
	}

	// Create client
	client := pluginv1.NewValidatorPluginClient(conn)

	// Fetch plugin info
	info, err := l.fetchInfo(client, cfg.GetTimeout(defaultGRPCTimeout))
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
func (l *GRPCLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

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
func (l *GRPCLoader) getOrCreateConnection(address string) (*grpc.ClientConn, error) {
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

	// Double-check after acquiring write lock
	if existingConn, exists := l.connections[address]; exists {
		return existingConn, nil
	}

	// Create new connection (lazy, non-blocking)
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create gRPC client for %s", address)
	}

	l.connections[address] = conn

	return conn, nil
}

// fetchInfo fetches plugin metadata via gRPC.
func (*GRPCLoader) fetchInfo(
	client pluginv1.ValidatorPluginClient,
	timeout time.Duration,
) (plugin.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Info(ctx, &pluginv1.InfoRequest{})
	if err != nil {
		return plugin.Info{}, errors.Wrapf(ErrGRPCInfoFailed, "gRPC error: %v", err)
	}

	return plugin.Info{
		Name:        resp.GetName(),
		Version:     resp.GetVersion(),
		Description: resp.GetDescription(),
		Author:      resp.GetAuthor(),
		URL:         resp.GetUrl(),
	}, nil
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
