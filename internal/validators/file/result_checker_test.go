package file_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
)

var _ = Describe("ChecksToolResult", func() {
	It("is reported by validators that read the file after the tool", func() {
		for _, v := range []validator.ResultChecker{
			&file.MarkdownValidator{},
			&file.TerraformValidator{},
			&file.GofumptValidator{},
			&file.WorkflowValidator{},
			&file.ShellScriptValidator{},
			&file.PythonValidator{},
			&file.JavaScriptValidator{},
			&file.RustValidator{},
		} {
			Expect(v.ChecksToolResult()).To(BeTrue())
		}
	})
})
