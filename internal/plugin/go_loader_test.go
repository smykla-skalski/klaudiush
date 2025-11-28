package plugin_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/pkg/config"
	pluginapi "github.com/smykla-labs/klaudiush/pkg/plugin"
)

// mockGoPlugin implements the public plugin.Plugin interface for testing.
type mockGoPlugin struct {
	info         pluginapi.Info
	validateFunc func(*pluginapi.ValidateRequest) *pluginapi.ValidateResponse
}

func (m *mockGoPlugin) Info() pluginapi.Info {
	return m.info
}

func (m *mockGoPlugin) Validate(req *pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
	if m.validateFunc != nil {
		return m.validateFunc(req)
	}

	return pluginapi.PassResponse()
}

var _ = Describe("GoLoader", func() {
	var (
		loader      *plugin.GoLoader
		tmpDir      string
		pluginDir   string
		projectRoot string
	)

	BeforeEach(func() {
		loader = plugin.NewGoLoader()

		// Create temp project structure
		var err error
		tmpDir, err = os.MkdirTemp("", "go-loader-test-*")
		Expect(err).NotTo(HaveOccurred())
		projectRoot = tmpDir

		pluginDir = filepath.Join(tmpDir, ".klaudiush", "plugins")
		err = os.MkdirAll(pluginDir, 0o755)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	})

	Describe("NewGoLoader", func() {
		It("should create a new loader", func() {
			Expect(loader).NotTo(BeNil())
		})
	})

	Describe("Load", func() {
		Context("with invalid configuration", func() {
			It("should return error when path is empty", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test",
					Type: config.PluginTypeGo,
					Path: "",
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("path is required"))
			})

			It("should return error when .so file does not exist", func() {
				pluginPath := filepath.Join(pluginDir, "nonexistent.so")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeGo,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to open Go plugin"))
			})

			It("should return error when extension is not .so", func() {
				pluginPath := filepath.Join(pluginDir, "plugin.dll")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeGo,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid Go plugin extension"))
			})

			It("should return error when path is not in allowed directory", func() {
				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeGo,
					Path:        "/tmp/plugin.so",
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("plugin path validation failed"))
			})

			It("should return error when path contains traversal patterns", func() {
				pluginPath := filepath.Join(pluginDir, "..", "..", "etc", "plugin.so")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeGo,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("plugin path validation failed"))
			})
		})

		// Note: Testing successful loading of .so files requires building actual
		// Go plugins, which is integration test territory. These unit tests focus
		// on error paths and adapter behavior.
	})

	Describe("Close", func() {
		It("should not return error", func() {
			err := loader.Close()

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("goPluginAdapter", func() {
		var (
			mockPlugin *mockGoPlugin
			adapter    plugin.Plugin
			ctx        context.Context
		)

		BeforeEach(func() {
			mockPlugin = &mockGoPlugin{
				info: pluginapi.Info{
					Name:        "test-plugin",
					Version:     "1.0.0",
					Description: "Test plugin",
				},
			}

			// Create adapter directly for testing
			// In real usage, this is done by GoLoader.Load()
			adapter = plugin.NewGoPluginAdapterForTesting(mockPlugin, nil)
			ctx = context.Background()
		})

		Describe("Info", func() {
			It("should return plugin info", func() {
				info := adapter.Info()

				Expect(info.Name).To(Equal("test-plugin"))
				Expect(info.Version).To(Equal("1.0.0"))
				Expect(info.Description).To(Equal("Test plugin"))
			})
		})

		Describe("Validate", func() {
			It("should call plugin's Validate method", func() {
				called := false
				mockPlugin.validateFunc = func(_ *pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
					called = true

					return pluginapi.PassResponse()
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				resp, err := adapter.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(called).To(BeTrue())
			})

			It("should inject plugin config into request", func() {
				adapterWithConfig := plugin.NewGoPluginAdapterForTesting(mockPlugin, map[string]any{
					"key1": "value1",
					"key2": 42,
				})

				var capturedRequest *pluginapi.ValidateRequest

				mockPlugin.validateFunc = func(req *pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
					capturedRequest = req

					return pluginapi.PassResponse()
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapterWithConfig.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedRequest.Config).To(HaveKeyWithValue("key1", "value1"))
				Expect(capturedRequest.Config).To(HaveKeyWithValue("key2", 42))
			})

			It("should not override existing config in request", func() {
				adapterWithConfig := plugin.NewGoPluginAdapterForTesting(mockPlugin, map[string]any{
					"key1": "value1",
				})

				var capturedRequest *pluginapi.ValidateRequest

				mockPlugin.validateFunc = func(req *pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
					capturedRequest = req

					return pluginapi.PassResponse()
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
					Config: map[string]any{
						"existing": "value",
					},
				}

				_, err := adapterWithConfig.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedRequest.Config).To(HaveKeyWithValue("existing", "value"))
				Expect(capturedRequest.Config).NotTo(HaveKey("key1"))
			})

			It("should respect cancelled context", func() {
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapter.Validate(cancelledCtx, req)

				Expect(err).To(MatchError(context.Canceled))
			})

			It("should return error when plugin returns nil response", func() {
				mockPlugin.validateFunc = func(*pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
					return nil
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapter.Validate(ctx, req)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("plugin returned nil response")))
			})

			It("should pass through plugin response", func() {
				mockPlugin.validateFunc = func(*pluginapi.ValidateRequest) *pluginapi.ValidateResponse {
					return pluginapi.FailResponse("test failure")
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				resp, err := adapter.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeTrue())
				Expect(resp.Message).To(Equal("test failure"))
			})
		})

		Describe("Close", func() {
			It("should not return error", func() {
				err := adapter.Close()

				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
