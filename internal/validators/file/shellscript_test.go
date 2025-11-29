package file_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("ShellScriptValidator", func() {
	var (
		v   *file.ShellScriptValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		// Create a real ShellChecker for integration tests
		runner := execpkg.NewCommandRunner(10 * time.Second)
		checker := linters.NewShellChecker(runner)
		v = file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	Describe("valid shell scripts", func() {
		It("should pass for valid bash script", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
echo "Hello, World!"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for valid sh script", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/sh
echo "Hello, World!"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("invalid shell scripts", func() {
		It("should fail for undefined variable", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
echo $UNDEFINED_VAR
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Shellcheck validation failed"))
		})

		It("should fail for syntax error", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/bash
if [ -f file.txt ]
  echo "File exists"
fi
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Shellcheck validation failed"))
		})
	})

	Describe("Fish scripts", func() {
		It("should skip .fish extension", func() {
			ctx.ToolInput.FilePath = "test.fish"
			ctx.ToolInput.Content = `#!/usr/bin/env fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/usr/bin/env fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip /usr/bin/fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/usr/bin/fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip /bin/fish shebang", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = `#!/bin/fish
echo "Hello from Fish"
`
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("edge cases", func() {
		It("should pass when no file path provided", func() {
			ctx.ToolInput.FilePath = ""
			ctx.ToolInput.Content = ""
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for empty content", func() {
			ctx.ToolInput.FilePath = "test.sh"
			ctx.ToolInput.Content = ""
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("fragment edits", func() {
		var (
			tmpDir  string
			tmpFile string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "shellscript-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("should pass when editing middle of bash script (shebang preserved)", func() {
			// Create a valid shell script file
			tmpFile = filepath.Join(tmpDir, "script.sh")
			originalContent := `#!/bin/bash

# Configuration
MAX_AGE=30
REGION="us-east-1"

# Main logic
echo "Starting cleanup..."
cleanup_resources() {
    echo "Cleaning up resources older than $MAX_AGE days in $REGION"
}
cleanup_resources
`
			err := os.WriteFile(tmpFile, []byte(originalContent), 0o644)
			Expect(err).NotTo(HaveOccurred())

			// Set up Edit operation for a fragment in the middle
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tmpFile
			ctx.ToolInput.OldString = `echo "Cleaning up resources older than $MAX_AGE days in $REGION"`
			ctx.ToolInput.NewString = `echo "Cleaning up resources older than ${MAX_AGE} days in ${REGION}"`

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass when editing fragment with unused variable (SC2034 excluded)", func() {
			// Create a script where the edited fragment defines variables used elsewhere
			tmpFile = filepath.Join(tmpDir, "script.sh")
			originalContent := `#!/bin/bash

# Configuration section
CONFIG_DIR="/etc/myapp"
LOG_LEVEL="debug"

# Later sections use these variables
main() {
    setup_logging
    run_app
}

main
`
			err := os.WriteFile(tmpFile, []byte(originalContent), 0o644)
			Expect(err).NotTo(HaveOccurred())

			// Edit just the config section - SC2034 would normally fire but should be excluded
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tmpFile
			ctx.ToolInput.OldString = `LOG_LEVEL="debug"`
			ctx.ToolInput.NewString = `LOG_LEVEL="info"`

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should still fail for real errors in fragments", func() {
			// Create a script with valid content
			tmpFile = filepath.Join(tmpDir, "script.sh")
			originalContent := `#!/bin/bash

echo "Starting..."

function cleanup() {
    echo "Cleaning up"
}

cleanup
`
			err := os.WriteFile(tmpFile, []byte(originalContent), 0o644)
			Expect(err).NotTo(HaveOccurred())

			// Introduce a real syntax error in the fragment
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tmpFile
			ctx.ToolInput.OldString = `function cleanup() {
    echo "Cleaning up"
}`
			ctx.ToolInput.NewString = `function cleanup() {
    if [ -f /tmp/test ]
        echo "Cleaning up"
    fi
}`

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Shellcheck validation failed"))
		})

		It("should detect shell type from env shebang", func() {
			tmpFile = filepath.Join(tmpDir, "script.sh")
			originalContent := `#!/usr/bin/env bash

readonly MYVAR="value"

process_data() {
    local data="$1"
    echo "$data"
}

process_data "test"
`
			err := os.WriteFile(tmpFile, []byte(originalContent), 0o644)
			Expect(err).NotTo(HaveOccurred())

			// Edit in the middle - should still recognize bash from env shebang
			ctx.ToolName = hook.ToolTypeEdit
			ctx.ToolInput.FilePath = tmpFile
			ctx.ToolInput.OldString = `local data="$1"`
			ctx.ToolInput.NewString = `local -r data="$1"`

			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It(
			"should pass when editing fragment with incomplete control structure (SC1089 excluded)",
			func() {
				// Create a script with if/else/fi structure
				tmpFile = filepath.Join(tmpDir, "script.sh")
				originalContent := `#!/bin/bash

cleanup_resources() {
    local resource="$1"
    if [[ -z "$resource" ]]; then
        echo "No resource specified"
        return 1
    else
        echo "Cleaning up $resource"
        rm -rf "$resource"
    fi
}

cleanup_resources "$@"
`
				err := os.WriteFile(tmpFile, []byte(originalContent), 0o644)
				Expect(err).NotTo(HaveOccurred())

				// Edit just the else block - fragment will contain partial control structure
				// Without SC1089 exclusion, this would fail with "Parsing stopped here"
				ctx.ToolName = hook.ToolTypeEdit
				ctx.ToolInput.FilePath = tmpFile
				ctx.ToolInput.OldString = `    else
        echo "Cleaning up $resource"
        rm -rf "$resource"`
				ctx.ToolInput.NewString = `    else
        echo "Cleaning up resource: $resource"
        rm -rf "$resource"`

				result := v.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			},
		)
	})

	Describe("config exclude rules", func() {
		It("should exclude rules specified in config", func() {
			// SC2086: Double quote to prevent globbing and word splitting
			// This script would fail without the exclusion
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewShellChecker(runner)
			cfg := &config.ShellScriptValidatorConfig{
				ExcludeRules: []string{"SC2086"},
			}
			validator := file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "test.sh",
					Content: `#!/bin/bash
VAR="hello world"
echo $VAR
`,
				},
			}

			result := validator.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should exclude rules with SC prefix", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewShellChecker(runner)
			cfg := &config.ShellScriptValidatorConfig{
				ExcludeRules: []string{"SC2086", "SC2154"},
			}
			validator := file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "test.sh",
					Content: `#!/bin/bash
echo $UNDEFINED_VAR
echo $ANOTHER_VAR
`,
				},
			}

			result := validator.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should exclude rules without SC prefix", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewShellChecker(runner)
			cfg := &config.ShellScriptValidatorConfig{
				ExcludeRules: []string{"2086"},
			}
			validator := file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "test.sh",
					Content: `#!/bin/bash
VAR="hello world"
echo $VAR
`,
				},
			}

			result := validator.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should still fail for non-excluded rules", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewShellChecker(runner)
			cfg := &config.ShellScriptValidatorConfig{
				ExcludeRules: []string{"SC2086"}, // Only exclude 2086
			}
			validator := file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "test.sh",
					Content: `#!/bin/bash
if [ -f file.txt ]
  echo "File exists"
fi
`,
				},
			}

			result := validator.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
		})

		It("should ignore invalid rule codes", func() {
			runner := execpkg.NewCommandRunner(10 * time.Second)
			checker := linters.NewShellChecker(runner)
			cfg := &config.ShellScriptValidatorConfig{
				ExcludeRules: []string{"invalid", "SC2086", "notanumber"},
			}
			validator := file.NewShellScriptValidator(logger.NewNoOpLogger(), checker, cfg, nil)

			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{
					FilePath: "test.sh",
					Content: `#!/bin/bash
VAR="hello world"
echo $VAR
`,
				},
			}

			// Should still work with valid rule (SC2086)
			result := validator.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
