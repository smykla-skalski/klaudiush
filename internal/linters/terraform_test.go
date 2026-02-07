package linters_test

import (
	"context"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
)

var (
	errTerraformFailed  = errors.New("terraform fmt failed")
	errTempFileCreation = errors.New("failed to create temp file")
)

var _ = Describe("TerraformFormatter", func() {
	var (
		ctrl            *gomock.Controller
		mockRunner      *execpkg.MockCommandRunner
		mockToolChecker *execpkg.MockToolChecker
		mockTempManager *execpkg.MockTempFileManager
		formatter       linters.TerraformFormatter
		ctx             context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockRunner = execpkg.NewMockCommandRunner(ctrl)
		mockToolChecker = execpkg.NewMockToolChecker(ctrl)
		mockTempManager = execpkg.NewMockTempFileManager(ctrl)
		ctx = context.Background()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("DetectTool", func() {
		BeforeEach(func() {
			formatter = linters.NewTerraformFormatterWithDeps(
				mockRunner,
				mockToolChecker,
				mockTempManager,
			)
		})

		Context("when tofu is available", func() {
			It("should return tofu", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("tofu")

				result := formatter.DetectTool()

				Expect(result).To(Equal("tofu"))
			})
		})

		Context("when only terraform is available", func() {
			It("should return terraform", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("terraform")

				result := formatter.DetectTool()

				Expect(result).To(Equal("terraform"))
			})
		})

		Context("when neither tool is available", func() {
			It("should return empty string", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("")

				result := formatter.DetectTool()

				Expect(result).To(Equal(""))
			})
		})
	})

	Describe("CheckFormat", func() {
		BeforeEach(func() {
			formatter = linters.NewTerraformFormatterWithDeps(
				mockRunner,
				mockToolChecker,
				mockTempManager,
			)
		})

		Context("when terraform fmt succeeds", func() {
			It("should return success", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("terraform")
				mockTempManager.EXPECT().Create("terraform-*.tf", gomock.Any()).
					Return("/tmp/terraform-123.tf", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "terraform", "fmt", "-check", "-diff", "/tmp/terraform-123.tf").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   "",
						ExitCode: 0,
					})

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

				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("terraform")
				mockTempManager.EXPECT().Create("terraform-*.tf", gomock.Any()).
					Return("/tmp/terraform-123.tf", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "terraform", "fmt", "-check", "-diff", "/tmp/terraform-123.tf").
					Return(execpkg.CommandResult{
						Stdout:   diffOutput,
						Stderr:   "",
						ExitCode: 3,
						Err:      errTerraformFailed,
					})

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

				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("terraform")
				mockTempManager.EXPECT().Create("terraform-*.tf", gomock.Any()).
					Return("/tmp/terraform-123.tf", func() {}, nil)
				mockRunner.EXPECT().
					Run(ctx, "terraform", "fmt", "-check", "-diff", "/tmp/terraform-123.tf").
					Return(execpkg.CommandResult{
						Stdout:   "",
						Stderr:   stderrOutput,
						ExitCode: 1,
						Err:      errTerraformFailed,
					})

				result := formatter.CheckFormat(ctx, "invalid terraform syntax")

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.RawOut).To(Equal(stderrOutput))
			})
		})

		Context("when neither terraform nor tofu is available", func() {
			It("should return success without validation", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("")

				result := formatter.CheckFormat(ctx, `resource "aws_instance" "example" {}`)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})

		Context("when temp file creation fails", func() {
			It("should return failure", func() {
				mockToolChecker.EXPECT().FindTool("tofu", "terraform").Return("terraform")
				mockTempManager.EXPECT().Create("terraform-*.tf", gomock.Any()).
					Return("", nil, errTempFileCreation)

				result := formatter.CheckFormat(ctx, `resource "aws_instance" "example" {}`)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(result.Err).To(HaveOccurred())
			})
		})
	})
})
