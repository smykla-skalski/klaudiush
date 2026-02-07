package plugin_test

import (
	"context"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/smykla-labs/klaudiush/api/plugin/v1"
	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/pkg/config"
	pluginpkg "github.com/smykla-labs/klaudiush/pkg/plugin"
)

var _ = Describe("GRPCLoader", func() {
	var (
		loader     *plugin.GRPCLoader
		mockServer *mockGRPCServer
		serverAddr string
	)

	BeforeEach(func() {
		loader = plugin.NewGRPCLoader()

		// Start mock gRPC server
		mockServer = newMockGRPCServer()
		serverAddr = mockServer.start()
	})

	AfterEach(func() {
		if loader != nil {
			_ = loader.Close()
		}

		if mockServer != nil {
			mockServer.stop()
		}
	})

	Describe("Load", func() {
		Context("when configuration is valid", func() {
			It("should load plugin successfully", func() {
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
				}

				p, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())

				info := p.Info()
				Expect(info.Name).To(Equal("mock-plugin"))
				Expect(info.Version).To(Equal("1.0.0"))
			})

			It("should reuse connections for the same address", func() {
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
				}

				p1, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(p1).NotTo(BeNil())

				p2, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(p2).NotTo(BeNil())

				// Both plugins should have been created (connections pooled internally)
				Expect(p1).NotTo(BeIdenticalTo(p2))
			})

			It("should pass plugin config to requests", func() {
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					Config: map[string]any{
						"key1": "value1",
						"key2": 42,
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())

				resp, err := p.Validate(context.Background(), &pluginpkg.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Passed).To(BeTrue())
			})
		})

		Context("when configuration is invalid", func() {
			It("should return error when address is missing", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test-plugin",
					Type: config.PluginTypeGRPC,
				}

				p, err := loader.Load(cfg)
				Expect(err).To(MatchError(plugin.ErrGRPCAddressRequired))
				Expect(p).To(BeNil())
			})

			It("should return error when connection fails", func() {
				timeoutLoader := plugin.NewGRPCLoaderWithTimeout(100 * time.Millisecond)
				defer timeoutLoader.Close()

				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: "localhost:99999", // Invalid port
				}

				p, err := timeoutLoader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(p).To(BeNil())
			})
		})

		Context("when Info RPC fails", func() {
			It("should return error", func() {
				mockServer.infoError = status.Error(codes.Internal, "info failed")

				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to fetch plugin info"))
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("Validate", func() {
		var (
			testPlugin plugin.Plugin
			cfg        *config.PluginInstanceConfig
		)

		BeforeEach(func() {
			cfg = &config.PluginInstanceConfig{
				Name:    "test-plugin",
				Type:    config.PluginTypeGRPC,
				Address: serverAddr,
			}

			var err error

			testPlugin, err = loader.Load(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate successfully", func() {
			resp, err := testPlugin.Validate(context.Background(), &pluginpkg.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
				Command:   "echo test",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Passed).To(BeTrue())
		})

		It("should return validation failure", func() {
			mockServer.validateResponse = &pluginv1.ValidateResponse{
				Passed:      false,
				ShouldBlock: true,
				Message:     "validation failed",
				ErrorCode:   "TEST001",
			}

			resp, err := testPlugin.Validate(context.Background(), &pluginpkg.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Passed).To(BeFalse())
			Expect(resp.ShouldBlock).To(BeTrue())
			Expect(resp.Message).To(Equal("validation failed"))
			Expect(resp.ErrorCode).To(Equal("TEST001"))
		})

		It("should handle timeout", func() {
			mockServer.validateDelay = 200 * time.Millisecond

			cfg.Timeout = config.Duration(50 * time.Millisecond)

			p, err := loader.Load(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = p.Validate(context.Background(), &pluginpkg.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("context deadline exceeded"),
				ContainSubstring("DeadlineExceeded"),
			))
		})

		It("should respect context cancellation", func() {
			mockServer.validateDelay = 200 * time.Millisecond

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			_, err := testPlugin.Validate(ctx, &pluginpkg.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		It("should convert config map correctly", func() {
			cfg.Config = map[string]any{
				"string_val": "test",
				"int_val":    42,
				"bool_val":   true,
				"map_val": map[string]string{
					"nested": "value",
				},
			}

			p, err := loader.Load(cfg)
			Expect(err).NotTo(HaveOccurred())

			resp, err := p.Validate(context.Background(), &pluginpkg.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Passed).To(BeTrue())
		})
	})

	Describe("TLS configuration", func() {
		Context("with localhost address", func() {
			It("should use insecure credentials by default (nil TLS config)", func() {
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr, // localhost:XXXXX
					TLS:     nil,
				}

				p, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())
			})

			It("should use insecure credentials with explicit TLS disabled", func() {
				enabled := false
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					TLS: &config.TLSConfig{
						Enabled: &enabled,
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())
			})
		})

		Context("with remote address", func() {
			It("should error by default (nil TLS config)", func() {
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: "remote.example.com:50051",
					TLS:     nil,
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insecure connection to remote host"))
				Expect(p).To(BeNil())
			})

			It("should error with explicit TLS disabled", func() {
				enabled := false
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: "remote.example.com:50051",
					TLS: &config.TLSConfig{
						Enabled: &enabled,
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insecure connection to remote host"))
				Expect(p).To(BeNil())
			})

			It("should allow insecure with AllowInsecureRemote flag", func() {
				timeoutLoader := plugin.NewGRPCLoaderWithTimeout(100 * time.Millisecond)
				defer timeoutLoader.Close()

				enabled := false
				allowInsecure := true
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: "remote.example.com:50051",
					TLS: &config.TLSConfig{
						Enabled:             &enabled,
						AllowInsecureRemote: &allowInsecure,
					},
				}

				// Should not error on credentials build, but will fail to connect
				p, err := timeoutLoader.Load(cfg)
				Expect(err).To(HaveOccurred())
				// Error should be connection failure, not TLS configuration error
				Expect(err.Error()).To(ContainSubstring("failed to fetch plugin info"))
				Expect(p).To(BeNil())
			})
		})

		Context("with TLS enabled", func() {
			It("should error when CA file does not exist", func() {
				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					TLS: &config.TLSConfig{
						Enabled: &enabled,
						CAFile:  "/nonexistent/ca.pem",
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load CA certificate"))
				Expect(p).To(BeNil())
			})

			It("should error when client cert files do not exist", func() {
				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					TLS: &config.TLSConfig{
						Enabled:  &enabled,
						CertFile: "/nonexistent/cert.pem",
						KeyFile:  "/nonexistent/key.pem",
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load TLS certificate"))
				Expect(p).To(BeNil())
			})

			It("should error when only CertFile is provided without KeyFile", func() {
				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					TLS: &config.TLSConfig{
						Enabled:  &enabled,
						CertFile: "/path/to/cert.pem",
						// KeyFile missing
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(
					err.Error(),
				).To(ContainSubstring("both cert_file and key_file must be specified"))
				Expect(p).To(BeNil())
			})

			It("should error when only KeyFile is provided without CertFile", func() {
				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "test-plugin",
					Type:    config.PluginTypeGRPC,
					Address: serverAddr,
					TLS: &config.TLSConfig{
						Enabled: &enabled,
						KeyFile: "/path/to/key.pem",
						// CertFile missing
					},
				}

				p, err := loader.Load(cfg)
				Expect(err).To(HaveOccurred())
				Expect(
					err.Error(),
				).To(ContainSubstring("both cert_file and key_file must be specified"))
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("Close", func() {
		It("should close all connections", func() {
			cfg := &config.PluginInstanceConfig{
				Name:    "test-plugin",
				Type:    config.PluginTypeGRPC,
				Address: serverAddr,
			}

			_, err := loader.Load(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = loader.Close()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return ErrLoaderClosed after Close is called", func() {
			cfg := &config.PluginInstanceConfig{
				Name:    "test-plugin",
				Type:    config.PluginTypeGRPC,
				Address: serverAddr,
			}

			_, err := loader.Load(cfg)
			Expect(err).NotTo(HaveOccurred())

			err = loader.Close()
			Expect(err).NotTo(HaveOccurred())

			// After Close, Load should return ErrLoaderClosed
			_, err = loader.Load(cfg)
			Expect(err).To(MatchError(plugin.ErrLoaderClosed))
		})

		It("should handle closing when no connections exist", func() {
			err := loader.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// mockGRPCServer is a mock gRPC server for testing.
type mockGRPCServer struct {
	pluginv1.UnimplementedValidatorPluginServer

	server           *grpc.Server
	listener         net.Listener
	infoError        error
	validateResponse *pluginv1.ValidateResponse
	validateError    error
	validateDelay    time.Duration
}

func newMockGRPCServer() *mockGRPCServer {
	return &mockGRPCServer{
		validateResponse: &pluginv1.ValidateResponse{
			Passed:      true,
			ShouldBlock: false,
		},
	}
}

func (m *mockGRPCServer) start() string {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	m.listener = lis
	m.server = grpc.NewServer()

	pluginv1.RegisterValidatorPluginServer(m.server, m)

	go func() {
		_ = m.server.Serve(lis)
	}()

	return lis.Addr().String()
}

func (m *mockGRPCServer) stop() {
	if m.server != nil {
		m.server.Stop()
	}
}

func (m *mockGRPCServer) Info(
	context.Context,
	*pluginv1.InfoRequest,
) (*pluginv1.InfoResponse, error) {
	if m.infoError != nil {
		return nil, m.infoError
	}

	return &pluginv1.InfoResponse{
		Name:        "mock-plugin",
		Version:     "1.0.0",
		Description: "Mock gRPC plugin for testing",
		Author:      "Test Author",
		Url:         "https://klaudiu.sh",
	}, nil
}

func (m *mockGRPCServer) Validate(
	ctx context.Context,
	_ *pluginv1.ValidateRequest,
) (*pluginv1.ValidateResponse, error) {
	if m.validateDelay > 0 {
		select {
		case <-time.After(m.validateDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.validateError != nil {
		return nil, m.validateError
	}

	return m.validateResponse, nil
}
