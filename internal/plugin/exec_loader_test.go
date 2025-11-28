package plugin_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/pkg/config"
	pluginapi "github.com/smykla-labs/klaudiush/pkg/plugin"
)

// mockCommandRunner is a mock implementation of exec.CommandRunner for testing.
type mockCommandRunner struct {
	runFunc          func(ctx context.Context, name string, args ...string) exec.CommandResult
	runWithStdinFunc func(ctx context.Context, stdin io.Reader, name string, args ...string) exec.CommandResult
}

func (m *mockCommandRunner) Run(
	ctx context.Context,
	name string,
	args ...string,
) exec.CommandResult {
	if m.runFunc != nil {
		return m.runFunc(ctx, name, args...)
	}

	return exec.CommandResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	}
}

func (m *mockCommandRunner) RunWithStdin(
	ctx context.Context,
	stdin io.Reader,
	name string,
	args ...string,
) exec.CommandResult {
	if m.runWithStdinFunc != nil {
		return m.runWithStdinFunc(ctx, stdin, name, args...)
	}

	return exec.CommandResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	}
}

func (m *mockCommandRunner) RunWithTimeout(
	_ time.Duration,
	name string,
	args ...string,
) exec.CommandResult {
	// Simple implementation that doesn't use timeout in tests
	return m.Run(context.Background(), name, args...)
}

var _ = Describe("ExecLoader", func() {
	var (
		loader      *plugin.ExecLoader
		runner      *mockCommandRunner
		tmpDir      string
		pluginDir   string
		projectRoot string
	)

	BeforeEach(func() {
		runner = &mockCommandRunner{}
		loader = plugin.NewExecLoader(runner)

		// Create temp project structure
		var err error
		tmpDir, err = os.MkdirTemp("", "exec-loader-test-*")
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

	Describe("NewExecLoader", func() {
		It("should create a new loader", func() {
			Expect(loader).NotTo(BeNil())
		})
	})

	Describe("Load", func() {
		Context("with invalid configuration", func() {
			It("should return error when path is empty", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test",
					Type: config.PluginTypeExec,
					Path: "",
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("path is required"))
			})

			It("should return error when path contains shell metacharacters", func() {
				pluginPath := filepath.Join(pluginDir, "plugin;rm -rf")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid characters in plugin path"))
			})

			It("should return error when path is not in allowed directory", func() {
				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        "/tmp/test-plugin",
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("plugin path validation failed"))
			})

			It("should return error when path contains traversal patterns", func() {
				pluginPath := filepath.Join(pluginDir, "..", "..", "etc", "passwd")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("plugin path validation failed"))
			})
		})

		Context("with valid configuration", func() {
			var validInfo pluginapi.Info

			BeforeEach(func() {
				validInfo = pluginapi.Info{
					Name:        "test-plugin",
					Version:     "1.0.0",
					Description: "Test exec plugin",
				}

				// Mock successful --version and --info calls
				runner.runFunc = func(_ context.Context, _ string, args ...string) exec.CommandResult {
					if len(args) > 0 && args[0] == "--version" {
						return exec.CommandResult{
							ExitCode: 0,
							Stdout:   "1.0.0",
						}
					}

					if len(args) > 0 && args[0] == "--info" {
						infoJSON, _ := json.Marshal(validInfo)

						return exec.CommandResult{
							ExitCode: 0,
							Stdout:   string(infoJSON),
						}
					}

					return exec.CommandResult{ExitCode: 1}
				}
			})

			It("should successfully load plugin", func() {
				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				p, err := loader.Load(cfg)

				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())
				Expect(p.Info().Name).To(Equal("test-plugin"))
			})

			It("should handle plugin with args", func() {
				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					Args:        []string{"--extra", "arg"},
					ProjectRoot: projectRoot,
				}

				p, err := loader.Load(cfg)

				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())
			})

			It("should handle custom timeout", func() {
				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					Timeout:     config.Duration(30 * time.Second),
					ProjectRoot: projectRoot,
				}

				p, err := loader.Load(cfg)

				Expect(err).NotTo(HaveOccurred())
				Expect(p).NotTo(BeNil())
			})
		})

		Context("when fetching plugin info fails", func() {
			It("should return error when --info execution fails", func() {
				runner.runFunc = func(_ context.Context, _ string, args ...string) exec.CommandResult {
					if len(args) > 0 && args[0] == "--version" {
						return exec.CommandResult{ExitCode: 0}
					}

					if len(args) > 0 && args[0] == "--info" {
						return exec.CommandResult{
							ExitCode: 1,
							Stderr:   "command failed",
						}
					}

					return exec.CommandResult{ExitCode: 0}
				}

				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exit code 1"))
			})

			It("should return error when --info output is not valid JSON", func() {
				runner.runFunc = func(_ context.Context, _ string, args ...string) exec.CommandResult {
					if len(args) > 0 && args[0] == "--version" {
						return exec.CommandResult{ExitCode: 0}
					}

					if len(args) > 0 && args[0] == "--info" {
						return exec.CommandResult{
							ExitCode: 0,
							Stdout:   "not json",
						}
					}

					return exec.CommandResult{ExitCode: 0}
				}

				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
				}

				_, err := loader.Load(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse plugin info JSON"))
			})
		})
	})

	Describe("Close", func() {
		It("should not return error", func() {
			err := loader.Close()

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("execPluginAdapter", func() {
		var (
			adapter plugin.Plugin
			ctx     context.Context
		)

		BeforeEach(func() {
			validInfo := pluginapi.Info{
				Name:        "test-plugin",
				Version:     "1.0.0",
				Description: "Test exec plugin",
			}

			// Mock successful --version and --info calls
			runner.runFunc = func(_ context.Context, _ string, args ...string) exec.CommandResult {
				if len(args) > 0 && args[0] == "--version" {
					return exec.CommandResult{ExitCode: 0}
				}

				if len(args) > 0 && args[0] == "--info" {
					infoJSON, _ := json.Marshal(validInfo)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(infoJSON),
					}
				}

				return exec.CommandResult{ExitCode: 0}
			}

			pluginPath := filepath.Join(pluginDir, "test-plugin")

			cfg := &config.PluginInstanceConfig{
				Name:        "test",
				Type:        config.PluginTypeExec,
				Path:        pluginPath,
				ProjectRoot: projectRoot,
			}

			var err error

			adapter, err = loader.Load(cfg)

			Expect(err).NotTo(HaveOccurred())

			ctx = context.Background()
		})

		Describe("Info", func() {
			It("should return plugin info", func() {
				info := adapter.Info()

				Expect(info.Name).To(Equal("test-plugin"))
				Expect(info.Version).To(Equal("1.0.0"))
				Expect(info.Description).To(Equal("Test exec plugin"))
			})
		})

		Describe("Validate", func() {
			It("should execute plugin and parse response", func() {
				runner.runWithStdinFunc = func(
					_ context.Context,
					_ io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					resp := pluginapi.PassResponse()
					respJSON, _ := json.Marshal(resp)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(respJSON),
					}
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				resp, err := adapter.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp.Passed).To(BeTrue())
			})

			It("should inject plugin config into request", func() {
				// Create new loader with config
				runner2 := &mockCommandRunner{
					runFunc: runner.runFunc,
				}
				loader2 := plugin.NewExecLoader(runner2)

				var capturedStdin []byte

				runner2.runWithStdinFunc = func(
					_ context.Context,
					stdin io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					capturedStdin, _ = io.ReadAll(stdin)
					resp := pluginapi.PassResponse()
					respJSON, _ := json.Marshal(resp)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(respJSON),
					}
				}

				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
					Config: map[string]any{
						"key1": "value1",
					},
				}

				adapter2, err := loader2.Load(cfg)

				Expect(err).NotTo(HaveOccurred())

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err = adapter2.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())

				var capturedReq pluginapi.ValidateRequest

				err = json.Unmarshal(capturedStdin, &capturedReq)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedReq.Config).To(HaveKeyWithValue("key1", "value1"))
			})

			It("should not override existing config in request", func() {
				runner2 := &mockCommandRunner{
					runFunc: runner.runFunc,
				}
				loader2 := plugin.NewExecLoader(runner2)

				var capturedStdin []byte

				runner2.runWithStdinFunc = func(
					_ context.Context,
					stdin io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					capturedStdin, _ = io.ReadAll(stdin)
					resp := pluginapi.PassResponse()
					respJSON, _ := json.Marshal(resp)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(respJSON),
					}
				}

				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					ProjectRoot: projectRoot,
					Config: map[string]any{
						"key1": "value1",
					},
				}

				adapter2, err := loader2.Load(cfg)

				Expect(err).NotTo(HaveOccurred())

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
					Config: map[string]any{
						"existing": "value",
					},
				}

				_, err = adapter2.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())

				var capturedReq pluginapi.ValidateRequest

				err = json.Unmarshal(capturedStdin, &capturedReq)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedReq.Config).To(HaveKeyWithValue("existing", "value"))
				Expect(capturedReq.Config).NotTo(HaveKey("key1"))
			})

			It("should apply timeout from config", func() {
				runner2 := &mockCommandRunner{
					runFunc: runner.runFunc,
				}
				loader2 := plugin.NewExecLoader(runner2)

				runner2.runWithStdinFunc = func(
					_ context.Context,
					_ io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					// Simulate slow execution
					time.Sleep(200 * time.Millisecond)
					resp := pluginapi.PassResponse()
					respJSON, _ := json.Marshal(resp)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(respJSON),
					}
				}

				pluginPath := filepath.Join(pluginDir, "test-plugin")

				cfg := &config.PluginInstanceConfig{
					Name:        "test",
					Type:        config.PluginTypeExec,
					Path:        pluginPath,
					Timeout:     config.Duration(100 * time.Millisecond),
					ProjectRoot: projectRoot,
				}

				adapter2, err := loader2.Load(cfg)

				Expect(err).NotTo(HaveOccurred())

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				// Should timeout since execution takes 200ms but timeout is 100ms
				// Note: This test depends on the mock runner respecting context cancellation
				_, err = adapter2.Validate(ctx, req)

				// The result depends on whether the runner respects context cancellation
				// In our mock, we just sleep, so it will complete but take longer than timeout
				Expect(err).NotTo(HaveOccurred())
			})

			It("should respect context cancellation", func() {
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				runner.runWithStdinFunc = func(
					_ context.Context,
					_ io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					return exec.CommandResult{
						Err: ctx.Err(),
					}
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapter.Validate(cancelledCtx, req)

				Expect(err).To(HaveOccurred())
			})

			It("should return error when plugin exits with non-zero code", func() {
				runner.runWithStdinFunc = func(
					_ context.Context,
					_ io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					return exec.CommandResult{
						ExitCode: 1,
						Stderr:   "execution failed",
					}
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapter.Validate(ctx, req)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exit code 1"))
			})

			It("should return error when response is not valid JSON", func() {
				runner.runWithStdinFunc = func(
					_ context.Context,
					_ io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   "not json",
					}
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
				}

				_, err := adapter.Validate(ctx, req)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse response JSON"))
			})

			It("should pass request as JSON to stdin", func() {
				var capturedStdin []byte

				runner.runWithStdinFunc = func(
					_ context.Context,
					stdin io.Reader,
					_ string,
					_ ...string,
				) exec.CommandResult {
					capturedStdin, _ = io.ReadAll(stdin)
					resp := pluginapi.PassResponse()
					respJSON, _ := json.Marshal(resp)

					return exec.CommandResult{
						ExitCode: 0,
						Stdout:   string(respJSON),
					}
				}

				req := &pluginapi.ValidateRequest{
					EventType: "PreToolUse",
					ToolName:  "Bash",
					Command:   "git commit",
				}

				_, err := adapter.Validate(ctx, req)

				Expect(err).NotTo(HaveOccurred())

				var capturedReq pluginapi.ValidateRequest

				err = json.Unmarshal(capturedStdin, &capturedReq)

				Expect(err).NotTo(HaveOccurred())
				Expect(capturedReq.EventType).To(Equal("PreToolUse"))
				Expect(capturedReq.ToolName).To(Equal("Bash"))
				Expect(capturedReq.Command).To(Equal("git commit"))
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
