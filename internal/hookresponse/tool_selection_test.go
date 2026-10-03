package hookresponse_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
)

var _ = Describe("BuildGeminiToolSelection", func() {
	It("offers only the listed tools and lets the model answer without one", func() {
		data, err := json.Marshal(hookresponse.BuildGeminiToolSelection(
			"BeforeToolSelection", []string{"read_file", "run_shell_command"},
		))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(MatchJSON(`{"hookSpecificOutput":{
			"hookEventName":"BeforeToolSelection",
			"toolConfig":{"mode":"AUTO","allowedFunctionNames":["read_file","run_shell_command"]}}}`))
	})

	It(
		"disables every tool when none is allowed, since AUTO with no names restricts nothing",
		func() {
			data, err := json.Marshal(
				hookresponse.BuildGeminiToolSelection("BeforeToolSelection", nil),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(MatchJSON(`{"hookSpecificOutput":{
			"hookEventName":"BeforeToolSelection",
			"toolConfig":{"mode":"NONE","allowedFunctionNames":[]}}}`))
		},
	)
})
