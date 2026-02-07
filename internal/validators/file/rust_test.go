package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("RustValidator", func() {
	var (
		v           *file.RustValidator
		ctx         *hook.Context
		mockCtrl    *gomock.Controller
		mockChecker *linters.MockRustfmtChecker
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
		v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("valid Rust code", func() {
		It("should pass for properly formatted Rust code", func() {
			ctx.ToolInput.FilePath = "test.rs"
			ctx.ToolInput.Content = `fn main() {
    println!("Hello, World!");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for simple function", func() {
			ctx.ToolInput.FilePath = "lib.rs"
			ctx.ToolInput.Content = `pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_add() {
        assert_eq!(add(2, 2), 4);
    }
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for struct definition", func() {
			ctx.ToolInput.FilePath = "types.rs"
			ctx.ToolInput.Content = `pub struct User {
    pub name: String,
    pub age: u32,
}

impl User {
    pub fn new(name: String, age: u32) -> Self {
        Self { name, age }
    }
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("invalid Rust code", func() {
		It("should fail for poorly formatted code", func() {
			ctx.ToolInput.FilePath = "test.rs"
			ctx.ToolInput.Content = `fn main(){println!("Hello");}`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "Diff in test.rs at line 1:\n fn main() {\n     println!(\"Hello\");\n }\n",
					Findings: []linters.LintFinding{
						{
							File:     "<temp>",
							Line:     0,
							Column:   0,
							Severity: linters.SeverityError,
							Message:  "Rust code formatting issues detected",
							Rule:     "rustfmt",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Rust code formatting issues detected"))
			Expect(result.Reference).To(Equal(validator.RefRustfmtCheck))
		})

		It("should fail with custom error message", func() {
			ctx.ToolInput.FilePath = "lib.rs"
			ctx.ToolInput.Content = `pub fn add(a:i32,b:i32)->i32{a+b}`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "Formatting errors detected",
					Findings: []linters.LintFinding{
						{
							File:     "<temp>",
							Line:     0,
							Column:   0,
							Severity: linters.SeverityError,
							Message:  "Rust code formatting issues detected",
							Rule:     "rustfmt",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Formatting errors detected"))
		})
	})

	Describe("configuration", func() {
		It("should respect edition configuration", func() {
			cfg := &config.RustValidatorConfig{
				Edition: "2024",
			}
			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.ToolInput.FilePath = "test.rs"
			ctx.ToolInput.Content = `fn main() {
    println!("Hello");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, _ string, opts *linters.RustfmtOptions) {
					Expect(opts.Edition).To(Equal("2024"))
				}).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should respect rustfmt config path", func() {
			cfg := &config.RustValidatorConfig{
				RustfmtConfig: "/path/to/rustfmt.toml",
			}
			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.ToolInput.FilePath = "test.rs"
			ctx.ToolInput.Content = `fn main() {
    println!("Hello");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, _ string, opts *linters.RustfmtOptions) {
					Expect(opts.ConfigPath).To(Equal("/path/to/rustfmt.toml"))
				}).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Edit operations with fragments", func() {
		var tempFile string

		BeforeEach(func() {
			// Create a temporary file for edit testing
			tmpDir := GinkgoT().TempDir()
			tempFile = filepath.Join(tmpDir, "test.rs")

			content := `fn main() {
    // Print greeting
    println!("Hello, World!");
}
`
			err := os.WriteFile(tempFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate edit fragment", func() {
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    // Print greeting`
			ctx.ToolInput.NewString = `    // Display greeting message`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should validate edit with context lines", func() {
			cfg := &config.RustValidatorConfig{}
			contextLines := 3
			cfg.ContextLines = &contextLines
			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    // Print greeting
    println!("Hello, World!");`
			ctx.ToolInput.NewString = `    // Print greeting
    println!("Hello, Rust!");`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail for badly formatted edit", func() {
			ctx.EventType = hook.EventTypePreToolUse
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tempFile
			ctx.ToolInput.OldString = `    // Print greeting
    println!("Hello, World!");`
			ctx.ToolInput.NewString = `    // Print greeting
println!("Hello, World!");`

			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&linters.LintResult{
					Success: false,
					RawOut:  "Formatting issues in fragment",
					Findings: []linters.LintFinding{
						{
							File:     "<temp>",
							Line:     0,
							Column:   0,
							Severity: linters.SeverityError,
							Message:  "Rust code formatting issues detected",
							Rule:     "rustfmt",
						},
					},
				})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("edition auto-detection", func() {
		var tempDir string
		var rustFile string

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()
			rustFile = filepath.Join(tempDir, "src", "main.rs")
			err := os.MkdirAll(filepath.Dir(rustFile), 0o755)
			Expect(err).NotTo(HaveOccurred())

			content := `fn main() {
    println!("Hello");
}
`
			err = os.WriteFile(rustFile, []byte(content), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should auto-detect edition from Cargo.toml", func() {
			cargoToml := filepath.Join(tempDir, "Cargo.toml")
			cargoContent := `[package]
name = "test"
version = "0.1.0"
edition = "2018"

[dependencies]
`
			err := os.WriteFile(cargoToml, []byte(cargoContent), 0o600)
			Expect(err).NotTo(HaveOccurred())

			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

			ctx.ToolInput.FilePath = rustFile
			ctx.ToolInput.Content = `fn main() {
    println!("Hello");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, _ string, opts *linters.RustfmtOptions) {
					Expect(opts.Edition).To(Equal("2018"))
				}).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should default to 2021 when Cargo.toml not found", func() {
			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

			ctx.ToolInput.FilePath = rustFile
			ctx.ToolInput.Content = `fn main() {
    println!("Hello");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, _ string, opts *linters.RustfmtOptions) {
					Expect(opts.Edition).To(Equal("2021"))
				}).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should prefer config edition over auto-detected", func() {
			cargoToml := filepath.Join(tempDir, "Cargo.toml")
			cargoContent := `[package]
name = "test"
version = "0.1.0"
edition = "2018"
`
			err := os.WriteFile(cargoToml, []byte(cargoContent), 0o600)
			Expect(err).NotTo(HaveOccurred())

			cfg := &config.RustValidatorConfig{
				Edition: "2024",
			}
			mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
			v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

			ctx.ToolInput.FilePath = rustFile
			ctx.ToolInput.Content = `fn main() {
    println!("Hello");
}
`
			mockChecker.EXPECT().
				CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, _ string, opts *linters.RustfmtOptions) {
					Expect(opts.Edition).To(Equal("2024"))
				}).
				Return(&linters.LintResult{Success: true})

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("no file path", func() {
		It("should pass when no file path is provided", func() {
			ctx.ToolInput.FilePath = ""
			ctx.ToolInput.Content = `fn main() { println!("Hello"); }`

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("configuration options", func() {
		Context("isUseRustfmt", func() {
			It("should return true by default", func() {
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main() {
    println!("hello");
}
`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseRustfmt config when disabled", func() {
				useRustfmt := false
				cfg := &config.RustValidatorConfig{
					UseRustfmt: &useRustfmt,
				}
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main(){println!("hello");}`
				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should respect UseRustfmt config when enabled", func() {
				useRustfmt := true
				cfg := &config.RustValidatorConfig{
					UseRustfmt: &useRustfmt,
				}
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main() {
    println!("hello");
}
`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("timeout", func() {
			It("should use custom timeout from config", func() {
				cfg := &config.RustValidatorConfig{
					Timeout: config.Duration(30 * time.Second),
				}
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main() {
    println!("hello");
}
`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("context lines", func() {
			It("should use default context lines when not configured", func() {
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, nil, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main() {
    println!("hello");
}
`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should use custom context lines from config", func() {
				contextLines := 5
				cfg := &config.RustValidatorConfig{
					ContextLines: &contextLines,
				}
				mockChecker = linters.NewMockRustfmtChecker(mockCtrl)
				v = file.NewRustValidator(logger.NewNoOpLogger(), mockChecker, cfg, nil)

				ctx.ToolInput.FilePath = "test.rs"
				ctx.ToolInput.Content = `fn main() {
    println!("hello");
}
`

				mockChecker.EXPECT().
					CheckWithOptions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&linters.LintResult{Success: true})

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("Category", func() {
		It("should return CategoryIO", func() {
			Expect(v.Category()).To(Equal(validator.CategoryIO))
		})
	})
})
