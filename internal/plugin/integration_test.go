package plugin_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pb "github.com/smykla-labs/klaudiush/api/plugin/v1"
	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	pluginapi "github.com/smykla-labs/klaudiush/pkg/plugin"
)

// Integration tests reuse mockGRPCServer from grpc_loader_test.go.
// The mock server provides configurable behavior for testing different scenarios.

// createExecPlugin creates a temporary exec plugin script for testing.
func createExecPlugin(
	tmpDir string,
	name string,
	response *pluginapi.ValidateResponse,
) (string, error) {
	script := filepath.Join(tmpDir, name)

	info := pluginapi.Info{
		Name:        name,
		Version:     "1.0.0",
		Description: "Test exec plugin: " + name,
	}

	infoJSON := fmt.Sprintf(`{"name":"%s","version":"%s","description":"%s"}`,
		info.Name, info.Version, info.Description)

	respJSON := "null"
	if response != nil {
		respJSON = fmt.Sprintf(`{"passed":%t,"should_block":%t,"message":"%s"}`,
			response.Passed, response.ShouldBlock, response.Message)
	}

	content := fmt.Sprintf(`#!/bin/bash
# Integration test exec plugin

# Handle --info flag
if [ "$1" = "--info" ]; then
  echo '%s'
  exit 0
fi

# Handle --version flag
if [ "$1" = "--version" ]; then
  echo "1.0.0"
  exit 0
fi

# Handle validation request (read JSON from stdin, output validation response)
read -r input
echo '%s'
`, infoJSON, respJSON)

	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return "", err
	}

	return script, nil
}

var _ = Describe("Plugin Integration Tests", func() {
	var (
		log     logger.Logger
		tmpDir  string
		cleanup func()
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()

		var err error

		tmpDir, err = os.MkdirTemp("", "plugin-integration-test-*")
		Expect(err).NotTo(HaveOccurred())

		cleanup = func() {
			if tmpDir != "" {
				_ = os.RemoveAll(tmpDir)
			}
		}
	})

	AfterEach(func() {
		if cleanup != nil {
			cleanup()
		}
	})

	Describe("Exec Plugin Integration", func() {
		Context("with a simple pass plugin", func() {
			It("should load and execute successfully", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"pass-plugin",
					&pluginapi.ValidateResponse{
						Passed:      true,
						ShouldBlock: false,
						Message:     "Test passed",
					},
				)
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "pass-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(1))

				result := validators[0].Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("Test passed"))
			})
		})

		Context("with a failing plugin", func() {
			It("should return failure result", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"fail-plugin", &pluginapi.ValidateResponse{
						Passed:      false,
						ShouldBlock: true,
						Message:     "Test failed",
					})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "fail-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(1))

				result := validators[0].Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("Test failed"))
			})
		})

		Context("with event type predicate", func() {
			It("should only match specified event types", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"event-plugin", &pluginapi.ValidateResponse{
						Passed:  true,
						Message: "Event matched",
					})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "event-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Predicate: &config.PluginPredicate{
						EventTypes: []string{"PreToolUse"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				// Should match PreToolUse
				preToolCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}
				validators := registry.GetValidators(preToolCtx)
				Expect(validators).To(HaveLen(1))

				// Should not match PostToolUse
				postToolCtx := &hook.Context{
					EventType: hook.EventTypePostToolUse,
					ToolName:  hook.ToolTypeBash,
				}
				validators = registry.GetValidators(postToolCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with tool type predicate", func() {
			It("should only match specified tool types", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"tool-plugin", &pluginapi.ValidateResponse{
						Passed:  true,
						Message: "Tool matched",
					})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "tool-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Predicate: &config.PluginPredicate{
						ToolTypes: []string{"Write", "Edit"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				// Should match Write
				writeCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
				}
				validators := registry.GetValidators(writeCtx)
				Expect(validators).To(HaveLen(1))

				// Should not match Bash
				bashCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}
				validators = registry.GetValidators(bashCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with file pattern predicate", func() {
			It("should only match specified file patterns", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"file-plugin", &pluginapi.ValidateResponse{
						Passed:  true,
						Message: "File matched",
					})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "file-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Predicate: &config.PluginPredicate{
						FilePatterns: []string{"*.go", "*.tf"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				// Should match .go file
				goFileCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "main.go",
					},
				}
				validators := registry.GetValidators(goFileCtx)
				Expect(validators).To(HaveLen(1))

				// Should not match .txt file
				txtFileCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "notes.txt",
					},
				}
				validators = registry.GetValidators(txtFileCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with command pattern predicate", func() {
			It("should only match specified command patterns", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"cmd-plugin", &pluginapi.ValidateResponse{
						Passed:  true,
						Message: "Command matched",
					})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "cmd-plugin",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    pluginPath,
					Predicate: &config.PluginPredicate{
						CommandPatterns: []string{"^git commit", "^terraform apply"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				// Should match git commit
				gitCommitCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m 'test'",
					},
				}
				validators := registry.GetValidators(gitCommitCtx)
				Expect(validators).To(HaveLen(1))

				// Should not match git push
				gitPushCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git push origin main",
					},
				}
				validators = registry.GetValidators(gitPushCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with multiple plugins", func() {
			It("should load and match multiple plugins correctly", func() {
				plugin1Path, err := createExecPlugin(
					tmpDir,
					"plugin1",
					&pluginapi.ValidateResponse{
						Passed:  true,
						Message: "Plugin 1",
					},
				)
				Expect(err).NotTo(HaveOccurred())

				plugin2Path, err := createExecPlugin(
					tmpDir,
					"plugin2",
					&pluginapi.ValidateResponse{
						Passed:  true,
						Message: "Plugin 2",
					},
				)
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				cfg1 := &config.PluginInstanceConfig{
					Name:    "plugin1",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    plugin1Path,
					Predicate: &config.PluginPredicate{
						ToolTypes: []string{"Bash"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				cfg2 := &config.PluginInstanceConfig{
					Name:    "plugin2",
					Type:    config.PluginTypeExec,
					Enabled: &enabled,
					Path:    plugin2Path,
					Predicate: &config.PluginPredicate{
						ToolTypes: []string{"Write"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugin(cfg1)
				Expect(err).NotTo(HaveOccurred())

				err = registry.LoadPlugin(cfg2)
				Expect(err).NotTo(HaveOccurred())

				// Bash context should match only plugin1
				bashCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}
				validators := registry.GetValidators(bashCtx)
				Expect(validators).To(HaveLen(1))

				result := validators[0].Validate(context.Background(), bashCtx)
				Expect(result.Message).To(ContainSubstring("Plugin 1"))

				// Write context should match only plugin2
				writeCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "test.txt",
					},
				}
				validators = registry.GetValidators(writeCtx)
				Expect(validators).To(HaveLen(1))

				result = validators[0].Validate(context.Background(), writeCtx)
				Expect(result.Message).To(ContainSubstring("Plugin 2"))
			})
		})
	})

	Describe("gRPC Plugin Integration", func() {
		Context("with a simple pass plugin", func() {
			It("should load and execute successfully", func() {
				srv := newMockGRPCServer()
				srv.validateResponse = &pb.ValidateResponse{
					Passed:      true,
					ShouldBlock: false,
					Message:     "gRPC validation passed",
				}

				address := srv.start()
				defer srv.stop()

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "grpc-pass-plugin",
					Type:    config.PluginTypeGRPC,
					Enabled: &enabled,
					Address: address,
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err := registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(1))

				result := validators[0].Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("gRPC validation passed"))
			})
		})

		Context("with a failing plugin", func() {
			It("should return failure result", func() {
				srv := newMockGRPCServer()
				srv.validateResponse = &pb.ValidateResponse{
					Passed:      false,
					ShouldBlock: true,
					Message:     "gRPC validation failed",
					ErrorCode:   "TEST001",
					FixHint:     "Fix the issue",
					DocLink:     "https://errors.smyk.la/TEST001",
				}

				address := srv.start()
				defer srv.stop()

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "grpc-fail-plugin",
					Type:    config.PluginTypeGRPC,
					Enabled: &enabled,
					Address: address,
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err := registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(1))

				result := validators[0].Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("gRPC validation failed"))
				// Plugin's DocLink is preserved as-is (plugins manage their own error references)
				expected := validator.Reference("https://errors.smyk.la/TEST001")
				Expect(result.Reference).To(Equal(expected))
				Expect(result.FixHint).To(Equal("Fix the issue"))
			})
		})

		Context("with predicates", func() {
			It("should respect event type filtering", func() {
				srv := newMockGRPCServer()
				srv.validateResponse = &pb.ValidateResponse{
					Passed:  true,
					Message: "Matched",
				}

				address := srv.start()
				defer srv.stop()

				enabled := true
				cfg := &config.PluginInstanceConfig{
					Name:    "grpc-event-plugin",
					Type:    config.PluginTypeGRPC,
					Enabled: &enabled,
					Address: address,
					Predicate: &config.PluginPredicate{
						EventTypes: []string{"PreToolUse"},
						ToolTypes:  []string{"Write"},
					},
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err := registry.LoadPlugin(cfg)
				Expect(err).NotTo(HaveOccurred())

				// Should match
				matchCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "test.txt",
					},
				}
				validators := registry.GetValidators(matchCtx)
				Expect(validators).To(HaveLen(1))

				// Should not match (different tool type)
				noMatchCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}
				validators = registry.GetValidators(noMatchCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with connection pooling", func() {
			It("should reuse connections for same address", func() {
				srv := newMockGRPCServer()

				address := srv.start()
				defer srv.stop()

				enabled := true
				cfg1 := &config.PluginInstanceConfig{
					Name:    "grpc-plugin-1",
					Type:    config.PluginTypeGRPC,
					Enabled: &enabled,
					Address: address,
					Timeout: config.Duration(5 * time.Second),
				}

				cfg2 := &config.PluginInstanceConfig{
					Name:    "grpc-plugin-2",
					Type:    config.PluginTypeGRPC,
					Enabled: &enabled,
					Address: address, // Same address
					Timeout: config.Duration(5 * time.Second),
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				// Load two plugins with the same address
				err := registry.LoadPlugin(cfg1)
				Expect(err).NotTo(HaveOccurred())

				err = registry.LoadPlugin(cfg2)
				Expect(err).NotTo(HaveOccurred())

				// Both should work (connection pooling should handle this)
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(2))

				for _, v := range validators {
					result := v.Validate(context.Background(), hookCtx)
					Expect(result.Passed).To(BeTrue())
				}
			})
		})
	})

	Describe("Mixed Plugin Types", func() {
		It("should support exec and gRPC plugins together", func() {
			// Create exec plugin
			execPath, err := createExecPlugin(
				tmpDir,
				"exec-mixed",
				&pluginapi.ValidateResponse{
					Passed:  true,
					Message: "Exec plugin",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			// Create gRPC plugin
			srv := newMockGRPCServer()
			srv.validateResponse = &pb.ValidateResponse{
				Passed:  true,
				Message: "gRPC plugin",
			}

			address := srv.start()
			defer srv.stop()

			enabled := true
			execCfg := &config.PluginInstanceConfig{
				Name:    "exec-mixed",
				Type:    config.PluginTypeExec,
				Enabled: &enabled,
				Path:    execPath,
				Predicate: &config.PluginPredicate{
					ToolTypes: []string{"Bash"},
				},
				Timeout: config.Duration(5 * time.Second),
			}

			grpcCfg := &config.PluginInstanceConfig{
				Name:    "grpc-mixed",
				Type:    config.PluginTypeGRPC,
				Enabled: &enabled,
				Address: address,
				Predicate: &config.PluginPredicate{
					ToolTypes: []string{"Bash"},
				},
				Timeout: config.Duration(5 * time.Second),
			}

			registry := plugin.NewRegistry(log)
			defer registry.Close()

			err = registry.LoadPlugin(execCfg)
			Expect(err).NotTo(HaveOccurred())

			err = registry.LoadPlugin(grpcCfg)
			Expect(err).NotTo(HaveOccurred())

			// Both should match
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "echo test",
				},
			}

			validators := registry.GetValidators(hookCtx)
			Expect(validators).To(HaveLen(2))

			// Execute both
			results := make([]*pluginapi.ValidateResponse, 0)
			for _, v := range validators {
				result := v.Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeTrue())

				resp := &pluginapi.ValidateResponse{
					Passed:  result.Passed,
					Message: result.Message,
				}
				results = append(results, resp)
			}

			// Verify we got both messages
			messages := []string{results[0].Message, results[1].Message}
			Expect(messages).To(ContainElement(ContainSubstring("Exec plugin")))
			Expect(messages).To(ContainElement(ContainSubstring("gRPC plugin")))
		})
	})

	Describe("Plugin Lifecycle", func() {
		It("should properly close all plugins on registry close", func() {
			execPath, err := createExecPlugin(tmpDir, "lifecycle-exec", &pluginapi.ValidateResponse{
				Passed: true,
			})
			Expect(err).NotTo(HaveOccurred())

			srv := newMockGRPCServer()
			address := srv.start()
			defer srv.stop()

			enabled := true
			execCfg := &config.PluginInstanceConfig{
				Name:    "lifecycle-exec",
				Type:    config.PluginTypeExec,
				Enabled: &enabled,
				Path:    execPath,
				Timeout: config.Duration(5 * time.Second),
			}

			grpcCfg := &config.PluginInstanceConfig{
				Name:    "lifecycle-grpc",
				Type:    config.PluginTypeGRPC,
				Enabled: &enabled,
				Address: address,
				Timeout: config.Duration(5 * time.Second),
			}

			registry := plugin.NewRegistry(log)

			err = registry.LoadPlugin(execCfg)
			Expect(err).NotTo(HaveOccurred())

			err = registry.LoadPlugin(grpcCfg)
			Expect(err).NotTo(HaveOccurred())

			// Close should not error
			err = registry.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Configuration", func() {
		Context("with disabled plugin", func() {
			It("should not load disabled plugins", func() {
				pluginPath, err := createExecPlugin(
					tmpDir,
					"disabled", &pluginapi.ValidateResponse{
						Passed: true,
					})
				Expect(err).NotTo(HaveOccurred())

				disabled := false
				enabled := true
				pluginConfig := &config.PluginConfig{
					Enabled: &enabled,
					Plugins: []*config.PluginInstanceConfig{
						{
							Name:    "disabled",
							Type:    config.PluginTypeExec,
							Enabled: &disabled,
							Path:    pluginPath,
							Timeout: config.Duration(5 * time.Second),
						},
					},
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				// LoadPlugins should skip disabled plugins
				err = registry.LoadPlugins(pluginConfig)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(0))
			})
		})

		Context("with LoadPlugins batch loading", func() {
			It("should load multiple plugins from config", func() {
				plugin1Path, err := createExecPlugin(
					tmpDir,
					"batch1",
					&pluginapi.ValidateResponse{
						Passed: true,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				plugin2Path, err := createExecPlugin(tmpDir, "batch2", &pluginapi.ValidateResponse{
					Passed: true,
				})
				Expect(err).NotTo(HaveOccurred())

				enabled := true
				pluginConfig := &config.PluginConfig{
					Enabled: &enabled,
					Plugins: []*config.PluginInstanceConfig{
						{
							Name:    "batch1",
							Type:    config.PluginTypeExec,
							Enabled: &enabled,
							Path:    plugin1Path,
							Timeout: config.Duration(5 * time.Second),
						},
						{
							Name:    "batch2",
							Type:    config.PluginTypeExec,
							Enabled: &enabled,
							Path:    plugin2Path,
							Timeout: config.Duration(5 * time.Second),
						},
					},
				}

				registry := plugin.NewRegistry(log)
				defer registry.Close()

				err = registry.LoadPlugins(pluginConfig)
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)
				Expect(validators).To(HaveLen(2))
			})
		})
	})
})
