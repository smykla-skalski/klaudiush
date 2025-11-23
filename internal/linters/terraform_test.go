package linters_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/claude-hooks/internal/exec"
	"github.com/smykla-labs/claude-hooks/internal/linters"
)

var errTerraformFailed = errors.New("terraform fmt failed")

var _ = Describe("TerraformFormatter", func() {
	var (
		formatter  linters.TerraformFormatter
		mockRunner *mockCommandRunner
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockRunner = &mockCommandRunner{}
		formatter = linters.NewTerraformFormatter(mockRunner)
	})

	Describe("DetectTool", func() {
		Context("when tofu is available", func() {
			It("should return tofu", func() {
				// This test depends on the actual system PATH
				// For now we'll test the behavior indirectly through CheckFormat
				Skip("DetectTool depends on system PATH - tested indirectly")
			})
		})

		Context("when only terraform is available", func() {
			It("should return terraform", func() {
				Skip("DetectTool depends on system PATH - tested indirectly")
			})
		})

		Context("when neither tool is available", func() {
			It("should return empty string", func() {
				Skip("DetectTool depends on system PATH - tested indirectly")
			})
		})
	})

	Describe("CheckFormat", func() {
		Context("when terraform fmt succeeds", func() {
			It("should return success", func() {
				mockRunner.runFunc = func(_ context.Context, name string, args ...string) execpkg.CommandResult {
					// Expect either tofu or terraform
					Expect(name).To(Or(Equal("tofu"), Equal("terraform")))
					Expect(args).To(ContainElements("fmt", "-check", "-diff"))

					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					}
				}

				result := formatter.CheckFormat(ctx, `resource "aws_instance" "example" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}`)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when terraform fmt fails due to formatting issues", func() {
			It("should return failure with diff output", func() {
				diffOutput := `--- old/main.tf
+++ new/main.tf
@@ -1,3 +1,3 @@
 resource "aws_instance" "example" {
-ami = "ami-12345"
+  ami = "ami-12345"
 }`

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   diffOutput,
						Stderr:   "",
						ExitCode: 3,
						Err:      errTerraformFailed,
					}
				}

				result := formatter.CheckFormat(ctx, `resource "aws_instance" "example" {
ami = "ami-12345"
}`)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(ContainSubstring("ami = \"ami-12345\""))
				Expect(result.Err).To(Equal(errTerraformFailed))
			})
		})

		Context("when terraform command returns error", func() {
			It("should include stderr in output", func() {
				stderrOutput := "Error: Invalid Terraform configuration"

				mockRunner.runFunc = func(_ context.Context, _ string, _ ...string) execpkg.CommandResult {
					return execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errTerraformFailed,
					}
				}

				result := formatter.CheckFormat(ctx, "invalid terraform syntax")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when neither terraform nor tofu is available", func() {
			It("should return success without validation", func() {
				// If no tool is available, DetectTool returns ""
				// and CheckFormat returns success
				// This is tested by not setting up any mocks
				// The real implementation will detect no tool available

				// Create a new formatter that will detect no tool
				// Since we can't easily mock ToolChecker, we rely on
				// the system not having the tool or use integration tests
				Skip("Requires ToolChecker injection - behavior verified in integration tests")
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				// This test requires injecting a mock TempFileManager
				Skip("Requires TempFileManager injection support")
			})
		})
	})
})
