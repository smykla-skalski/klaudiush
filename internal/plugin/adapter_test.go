package plugin_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	pluginapi "github.com/smykla-labs/klaudiush/pkg/plugin"
)

var _ = Describe("ValidatorAdapter", func() {
	var (
		mockPlugin *plugin.MockPlugin
		adapter    validator.Validator
		log        logger.Logger
		ctx        context.Context
		ctrl       *gomock.Controller
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		mockPlugin = plugin.NewMockPlugin(ctrl)

		mockPlugin.EXPECT().Info().Return(pluginapi.Info{
			Name:    "test-plugin",
			Version: "1.0.0",
		}).AnyTimes()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("NewValidatorAdapter", func() {
		It("should create adapter with CPU category", func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryCPU, log)

			Expect(adapter).NotTo(BeNil())
			Expect(adapter.Name()).To(Equal("plugin:test-plugin"))
			Expect(adapter.Category()).To(Equal(validator.CategoryCPU))
		})

		It("should create adapter with IO category", func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryIO, log)

			Expect(adapter).NotTo(BeNil())
			Expect(adapter.Category()).To(Equal(validator.CategoryIO))
		})

		It("should create adapter with Git category", func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryGit, log)

			Expect(adapter).NotTo(BeNil())
			Expect(adapter.Category()).To(Equal(validator.CategoryGit))
		})
	})

	Describe("Validate", func() {
		BeforeEach(func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryCPU, log)
		})

		It("should convert hook context to plugin request", func() {
			var capturedRequest *pluginapi.ValidateRequest

			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context,
					req *pluginapi.ValidateRequest,
				) (*pluginapi.ValidateResponse, error) {
					capturedRequest = req

					return pluginapi.PassResponse(), nil
				})

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git commit -m 'test'",
				},
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())
			Expect(capturedRequest).NotTo(BeNil())
			Expect(capturedRequest.EventType).To(Equal("PreToolUse"))
			Expect(capturedRequest.ToolName).To(Equal("Bash"))
			Expect(capturedRequest.Command).To(Equal("git commit -m 'test'"))
		})

		It("should handle Write tool with file path and content", func() {
			var capturedRequest *pluginapi.ValidateRequest

			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context,
					req *pluginapi.ValidateRequest,
				) (*pluginapi.ValidateResponse, error) {
					capturedRequest = req

					return pluginapi.PassResponse(), nil
				})

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "/path/to/file.go",
					Content:  "package main",
				},
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(capturedRequest.FilePath).To(Equal("/path/to/file.go"))
			Expect(capturedRequest.Content).To(Equal("package main"))
		})

		It("should handle Edit tool with old and new strings", func() {
			var capturedRequest *pluginapi.ValidateRequest

			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context,
					req *pluginapi.ValidateRequest,
				) (*pluginapi.ValidateResponse, error) {
					capturedRequest = req

					return pluginapi.PassResponse(), nil
				})

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeEdit,
				ToolInput: hook.ToolInput{
					FilePath:  "/path/to/file.go",
					OldString: "old text",
					NewString: "new text",
				},
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(capturedRequest.OldString).To(Equal("old text"))
			Expect(capturedRequest.NewString).To(Equal("new text"))
		})

		It("should handle Grep tool with pattern", func() {
			var capturedRequest *pluginapi.ValidateRequest

			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context,
					req *pluginapi.ValidateRequest,
				) (*pluginapi.ValidateResponse, error) {
					capturedRequest = req

					return pluginapi.PassResponse(), nil
				})

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeGrep,
				ToolInput: hook.ToolInput{
					Pattern: "TODO",
				},
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(capturedRequest.Pattern).To(Equal("TODO"))
		})

		It("should convert pass response", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(pluginapi.PassResponse(), nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())
			Expect(result.ShouldBlock).To(BeFalse())
		})

		It("should convert fail response", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(pluginapi.FailResponse("validation failed"), nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("validation failed"))
		})

		It("should convert warn response", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(pluginapi.WarnResponse("warning message"), nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(Equal("warning message"))
		})

		It("should preserve plugin's DocLink as reference", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(pluginapi.FailWithCode(
					"TEST001",
					"validation failed",
					"fix hint",
					"https://errors.smyk.la/TEST001",
				), nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			// Plugin's own DocLink is used as-is, not converted to klaudiu.sh URL
			expected := validator.Reference("https://errors.smyk.la/TEST001")
			Expect(result.Reference).To(Equal(expected))
			Expect(result.FixHint).To(Equal("fix hint"))
		})

		It("should not set reference when plugin provides no DocLink", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(pluginapi.FailResponse("validation failed"), nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.Reference("")))
		})

		It("should preserve FixHint even without DocLink", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(&pluginapi.ValidateResponse{
					Passed:      false,
					ShouldBlock: true,
					Message:     "validation failed",
					FixHint:     "try this fix",
				}, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.FixHint).To(Equal("try this fix"))
			Expect(result.Reference).To(Equal(validator.Reference("")))
		})

		It("should convert details map", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(&pluginapi.ValidateResponse{
					Passed:      false,
					ShouldBlock: true,
					Message:     "validation failed",
					Details: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				}, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Details).To(HaveKeyWithValue("key1", "value1"))
			Expect(result.Details).To(HaveKeyWithValue("key2", "value2"))
		})

		It("should handle plugin errors", func() {
			mockPlugin.EXPECT().
				Validate(gomock.Any(), gomock.Any()).
				Return(nil, context.DeadlineExceeded)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
			}

			result := adapter.Validate(ctx, hookCtx)

			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(ContainSubstring("Plugin error"))
		})
	})

	Describe("Category", func() {
		It("should return configured category", func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryIO, log)

			Expect(adapter.Category()).To(Equal(validator.CategoryIO))
		})
	})

	Describe("Close", func() {
		BeforeEach(func() {
			adapter = plugin.NewValidatorAdapter(mockPlugin, validator.CategoryCPU, log)
		})

		It("should call plugin Close method", func() {
			mockPlugin.EXPECT().Close().Return(nil)

			closeableAdapter, ok := adapter.(interface{ Close() error })

			Expect(ok).To(BeTrue())

			err := closeableAdapter.Close()

			Expect(err).NotTo(HaveOccurred())
		})

		It("should propagate plugin Close errors", func() {
			expectedErr := context.DeadlineExceeded
			mockPlugin.EXPECT().Close().Return(expectedErr)

			closeableAdapter, ok := adapter.(interface{ Close() error })

			Expect(ok).To(BeTrue())

			err := closeableAdapter.Close()

			Expect(err).To(MatchError(expectedErr))
		})
	})
})
