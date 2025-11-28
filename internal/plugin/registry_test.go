package plugin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	pluginapi "github.com/smykla-labs/klaudiush/pkg/plugin"
)

var _ = Describe("Registry", func() {
	var (
		registry *plugin.Registry
		log      logger.Logger
		ctrl     *gomock.Controller
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		registry = plugin.NewRegistry(log)
		ctrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("NewRegistry", func() {
		It("should create a new registry", func() {
			Expect(registry).NotTo(BeNil())
		})
	})

	Describe("LoadPlugins", func() {
		Context("when config is nil", func() {
			It("should not return error", func() {
				err := registry.LoadPlugins(nil)

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when plugins are disabled globally", func() {
			It("should not load plugins", func() {
				cfg := &config.PluginConfig{
					Enabled: boolPtr(false),
				}

				err := registry.LoadPlugins(cfg)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(BeEmpty())
			})
		})

		Context("when individual plugin is disabled", func() {
			It("should skip disabled plugin", func() {
				cfg := &config.PluginConfig{
					Enabled: boolPtr(true),
					Plugins: []*config.PluginInstanceConfig{
						{
							Name:    "disabled-plugin",
							Type:    config.PluginTypeExec,
							Path:    "/usr/local/bin/test-plugin",
							Enabled: boolPtr(false),
						},
					},
				}

				err := registry.LoadPlugins(cfg)

				// Should not error, just skip
				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(BeEmpty())
			})
		})

		Context("when loading invalid plugin", func() {
			It("should return error for unsupported plugin type", func() {
				cfg := &config.PluginConfig{
					Enabled: boolPtr(true),
					Plugins: []*config.PluginInstanceConfig{
						{
							Name: "invalid-plugin",
							Type: config.PluginType("invalid"),
							Path: "/usr/local/bin/test-plugin",
						},
					},
				}

				err := registry.LoadPlugins(cfg)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported plugin type"))
			})
		})
	})

	Describe("GetValidators", func() {
		var mockPlugin *plugin.MockPlugin

		BeforeEach(func() {
			mockPlugin = plugin.NewMockPlugin(ctrl)
			mockPlugin.EXPECT().Info().Return(pluginapi.Info{
				Name:    "test-plugin",
				Version: "1.0.0",
			}).AnyTimes()
		})

		Context("with no plugins loaded", func() {
			It("should return empty list", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(BeEmpty())
			})
		})

		Context("with plugins that match", func() {
			It("should return matching validators", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test-plugin",
					Type: config.PluginTypeGo,
					Predicate: &config.PluginPredicate{
						EventTypes: []string{"PreToolUse"},
						ToolTypes:  []string{"Bash"},
					},
				}

				// Use mock plugin directly
				err := registry.LoadPluginForTesting(mockPlugin, cfg)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(HaveLen(1))
			})
		})

		Context("with plugins that don't match", func() {
			It("should not return validators for wrong event type", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test-plugin",
					Type: config.PluginTypeGo,
					Predicate: &config.PluginPredicate{
						EventTypes: []string{"PostToolUse"},
					},
				}

				err := registry.LoadPluginForTesting(mockPlugin, cfg)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(BeEmpty())
			})

			It("should not return validators for wrong tool type", func() {
				cfg := &config.PluginInstanceConfig{
					Name: "test-plugin",
					Type: config.PluginTypeGo,
					Predicate: &config.PluginPredicate{
						EventTypes: []string{"PreToolUse"},
						ToolTypes:  []string{"Write"},
					},
				}

				err := registry.LoadPluginForTesting(mockPlugin, cfg)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				validators := registry.GetValidators(hookCtx)

				Expect(validators).To(BeEmpty())
			})
		})
	})

	Describe("Close", func() {
		It("should not return error when no plugins loaded", func() {
			err := registry.Close()

			Expect(err).NotTo(HaveOccurred())
		})

		It("should close all loaded plugins", func() {
			mockPlugin := plugin.NewMockPlugin(ctrl)
			mockPlugin.EXPECT().Info().Return(pluginapi.Info{
				Name:    "test-plugin",
				Version: "1.0.0",
			}).AnyTimes()
			mockPlugin.EXPECT().Close().Return(nil).Times(1)

			cfg := &config.PluginInstanceConfig{
				Name: "test-plugin",
				Type: config.PluginTypeGo,
			}

			err := registry.LoadPluginForTesting(mockPlugin, cfg)

			Expect(err).NotTo(HaveOccurred())

			err = registry.Close()

			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("PredicateMatcher", func() {
	Describe("NewPredicateMatcher", func() {
		It("should create matcher with no filters", func() {
			matcher, err := plugin.NewPredicateMatcher(nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())
		})

		It("should create matcher with event type filter", func() {
			predicate := &config.PluginPredicate{
				EventTypes: []string{"PreToolUse", "PostToolUse"},
			}

			matcher, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())
		})

		It("should create matcher with tool type filter", func() {
			predicate := &config.PluginPredicate{
				ToolTypes: []string{"Bash", "Write"},
			}

			matcher, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())
		})

		It("should create matcher with file patterns", func() {
			predicate := &config.PluginPredicate{
				FilePatterns: []string{"*.go", "*.md"},
			}

			matcher, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())
		})

		It("should return error for invalid file pattern", func() {
			predicate := &config.PluginPredicate{
				FilePatterns: []string{"[invalid"},
			}

			_, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid file pattern"))
		})

		It("should create matcher with command patterns", func() {
			predicate := &config.PluginPredicate{
				CommandPatterns: []string{"^git commit", "^git push"},
			}

			matcher, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())
		})

		It("should return error for invalid regex pattern", func() {
			predicate := &config.PluginPredicate{
				CommandPatterns: []string{"[invalid"},
			}

			_, err := plugin.NewPredicateMatcher(predicate)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid command pattern"))
		})
	})

	Describe("Matches", func() {
		Context("with no filters", func() {
			It("should match any context", func() {
				matcher, _ := plugin.NewPredicateMatcher(nil)

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})
		})

		Context("with event type filter", func() {
			var matcher *plugin.PredicateMatcher

			BeforeEach(func() {
				predicate := &config.PluginPredicate{
					EventTypes: []string{"PreToolUse"},
				}

				var err error

				matcher, err = plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should match correct event type", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})

			It("should not match wrong event type", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePostToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})
		})

		Context("with tool type filter", func() {
			var matcher *plugin.PredicateMatcher

			BeforeEach(func() {
				predicate := &config.PluginPredicate{
					ToolTypes: []string{"Bash"},
				}

				var err error

				matcher, err = plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should match correct tool type", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})

			It("should not match wrong tool type", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})
		})

		Context("with file pattern filter", func() {
			var matcher *plugin.PredicateMatcher

			BeforeEach(func() {
				predicate := &config.PluginPredicate{
					FilePatterns: []string{"*.go", "*.md"},
				}

				var err error

				matcher, err = plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should match file that matches pattern", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "test.go",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})

			It("should not match file that doesn't match pattern", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "test.txt",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})

			It("should not match when tool is not a file tool", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})

			It("should not match when file path is empty", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})
		})

		Context("with command pattern filter", func() {
			var matcher *plugin.PredicateMatcher

			BeforeEach(func() {
				predicate := &config.PluginPredicate{
					CommandPatterns: []string{"^git commit"},
				}

				var err error

				matcher, err = plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should match command that matches pattern", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m 'test'",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})

			It("should not match command that doesn't match pattern", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git push",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})

			It("should not match when tool is not Bash", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})

			It("should not match when command is empty", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "",
					},
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})
		})

		Context("with multiple filters", func() {
			It("should match when all filters match", func() {
				predicate := &config.PluginPredicate{
					EventTypes: []string{"PreToolUse"},
					ToolTypes:  []string{"Bash"},
				}

				matcher, err := plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeTrue())
			})

			It("should not match when any filter doesn't match", func() {
				predicate := &config.PluginPredicate{
					EventTypes: []string{"PreToolUse"},
					ToolTypes:  []string{"Write"},
				}

				matcher, err := plugin.NewPredicateMatcher(predicate)

				Expect(err).NotTo(HaveOccurred())

				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				Expect(matcher.Matches(hookCtx)).To(BeFalse())
			})
		})
	})
})

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}
