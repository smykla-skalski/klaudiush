package plugin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

var _ = Describe("API", func() {
	Describe("Info", func() {
		It("should contain plugin metadata", func() {
			info := plugin.Info{
				Name:        "test-plugin",
				Version:     "1.0.0",
				Description: "A test plugin",
				Author:      "test-author",
				URL:         "https://klaudiu.sh",
			}

			Expect(info.Name).To(Equal("test-plugin"))
			Expect(info.Version).To(Equal("1.0.0"))
			Expect(info.Description).To(Equal("A test plugin"))
			Expect(info.Author).To(Equal("test-author"))
			Expect(info.URL).To(Equal("https://klaudiu.sh"))
		})

		It("should allow optional fields to be empty", func() {
			info := plugin.Info{
				Name:    "minimal-plugin",
				Version: "1.0.0",
			}

			Expect(info.Name).To(Equal("minimal-plugin"))
			Expect(info.Version).To(Equal("1.0.0"))
			Expect(info.Description).To(BeEmpty())
			Expect(info.Author).To(BeEmpty())
			Expect(info.URL).To(BeEmpty())
		})
	})

	Describe("ValidateRequest", func() {
		It("should contain hook context information", func() {
			req := &plugin.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
				Command:   "git commit -m 'test'",
				FilePath:  "/path/to/file",
				Content:   "file content",
				OldString: "old",
				NewString: "new",
				Pattern:   "*.go",
				Config: map[string]any{
					"key": "value",
				},
			}

			Expect(req.EventType).To(Equal("PreToolUse"))
			Expect(req.ToolName).To(Equal("Bash"))
			Expect(req.Command).To(Equal("git commit -m 'test'"))
			Expect(req.FilePath).To(Equal("/path/to/file"))
			Expect(req.Content).To(Equal("file content"))
			Expect(req.OldString).To(Equal("old"))
			Expect(req.NewString).To(Equal("new"))
			Expect(req.Pattern).To(Equal("*.go"))
			Expect(req.Config).To(HaveKeyWithValue("key", "value"))
		})

		It("should allow optional fields to be empty", func() {
			req := &plugin.ValidateRequest{
				EventType: "PreToolUse",
				ToolName:  "Bash",
			}

			Expect(req.EventType).To(Equal("PreToolUse"))
			Expect(req.ToolName).To(Equal("Bash"))
			Expect(req.Command).To(BeEmpty())
			Expect(req.FilePath).To(BeEmpty())
			Expect(req.Content).To(BeEmpty())
			Expect(req.Config).To(BeNil())
		})
	})

	Describe("ValidateResponse", func() {
		It("should contain validation result information", func() {
			resp := &plugin.ValidateResponse{
				Passed:      false,
				ShouldBlock: true,
				Message:     "Validation failed",
				ErrorCode:   "TEST001",
				FixHint:     "Fix the issue",
				DocLink:     "https://docs.klaudiu.sh/TEST001",
				Details: map[string]string{
					"reason": "test failure",
				},
			}

			Expect(resp.Passed).To(BeFalse())
			Expect(resp.ShouldBlock).To(BeTrue())
			Expect(resp.Message).To(Equal("Validation failed"))
			Expect(resp.ErrorCode).To(Equal("TEST001"))
			Expect(resp.FixHint).To(Equal("Fix the issue"))
			Expect(resp.DocLink).To(Equal("https://docs.klaudiu.sh/TEST001"))
			Expect(resp.Details).To(HaveKeyWithValue("reason", "test failure"))
		})

		Describe("AddDetail", func() {
			It("should add a detail entry", func() {
				resp := &plugin.ValidateResponse{}

				result := resp.AddDetail("key1", "value1")

				Expect(result).To(BeIdenticalTo(resp))
				Expect(resp.Details).To(HaveKeyWithValue("key1", "value1"))
			})

			It("should add multiple detail entries", func() {
				resp := &plugin.ValidateResponse{}

				resp.AddDetail("key1", "value1").
					AddDetail("key2", "value2").
					AddDetail("key3", "value3")

				Expect(resp.Details).To(HaveKeyWithValue("key1", "value1"))
				Expect(resp.Details).To(HaveKeyWithValue("key2", "value2"))
				Expect(resp.Details).To(HaveKeyWithValue("key3", "value3"))
			})

			It("should initialize Details map if nil", func() {
				resp := &plugin.ValidateResponse{
					Details: nil,
				}

				resp.AddDetail("key", "value")

				Expect(resp.Details).NotTo(BeNil())
				Expect(resp.Details).To(HaveKeyWithValue("key", "value"))
			})

			It("should preserve existing Details entries", func() {
				resp := &plugin.ValidateResponse{
					Details: map[string]string{
						"existing": "value",
					},
				}

				resp.AddDetail("new", "value")

				Expect(resp.Details).To(HaveKeyWithValue("existing", "value"))
				Expect(resp.Details).To(HaveKeyWithValue("new", "value"))
			})
		})
	})

	Describe("Response Helpers", func() {
		Describe("PassResponse", func() {
			It("should create a passing response", func() {
				resp := plugin.PassResponse()

				Expect(resp.Passed).To(BeTrue())
				Expect(resp.ShouldBlock).To(BeFalse())
				Expect(resp.Message).To(BeEmpty())
			})
		})

		Describe("PassWithMessage", func() {
			It("should create a passing response with message", func() {
				resp := plugin.PassWithMessage("All checks passed")

				Expect(resp.Passed).To(BeTrue())
				Expect(resp.ShouldBlock).To(BeFalse())
				Expect(resp.Message).To(Equal("All checks passed"))
			})
		})

		Describe("FailResponse", func() {
			It("should create a failing response that blocks", func() {
				resp := plugin.FailResponse("Validation failed")

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeTrue())
				Expect(resp.Message).To(Equal("Validation failed"))
			})
		})

		Describe("WarnResponse", func() {
			It("should create a failing response that does not block", func() {
				resp := plugin.WarnResponse("Warning message")

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeFalse())
				Expect(resp.Message).To(Equal("Warning message"))
			})
		})

		Describe("FailWithCode", func() {
			It("should create a failing response with code and hints", func() {
				resp := plugin.FailWithCode(
					"TEST001",
					"Validation failed",
					"Fix the issue",
					"https://docs.klaudiu.sh/TEST001",
				)

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeTrue())
				Expect(resp.Message).To(Equal("Validation failed"))
				Expect(resp.ErrorCode).To(Equal("TEST001"))
				Expect(resp.FixHint).To(Equal("Fix the issue"))
				Expect(resp.DocLink).To(Equal("https://docs.klaudiu.sh/TEST001"))
			})

			It("should handle empty hints and links", func() {
				resp := plugin.FailWithCode("TEST001", "Validation failed", "", "")

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeTrue())
				Expect(resp.Message).To(Equal("Validation failed"))
				Expect(resp.ErrorCode).To(Equal("TEST001"))
				Expect(resp.FixHint).To(BeEmpty())
				Expect(resp.DocLink).To(BeEmpty())
			})
		})

		Describe("WarnWithCode", func() {
			It("should create a warning response with code and hints", func() {
				resp := plugin.WarnWithCode(
					"TEST002",
					"Warning message",
					"Consider fixing",
					"https://docs.klaudiu.sh/TEST002",
				)

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeFalse())
				Expect(resp.Message).To(Equal("Warning message"))
				Expect(resp.ErrorCode).To(Equal("TEST002"))
				Expect(resp.FixHint).To(Equal("Consider fixing"))
				Expect(resp.DocLink).To(Equal("https://docs.klaudiu.sh/TEST002"))
			})

			It("should handle empty hints and links", func() {
				resp := plugin.WarnWithCode("TEST002", "Warning message", "", "")

				Expect(resp.Passed).To(BeFalse())
				Expect(resp.ShouldBlock).To(BeFalse())
				Expect(resp.Message).To(Equal("Warning message"))
				Expect(resp.ErrorCode).To(Equal("TEST002"))
				Expect(resp.FixHint).To(BeEmpty())
				Expect(resp.DocLink).To(BeEmpty())
			})
		})
	})
})
