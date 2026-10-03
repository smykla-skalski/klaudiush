package harness_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func denyFixture() harness.Fixture {
	return harness.Fixture{
		Provider:       hook.ProviderClaude,
		Harness:        "claude",
		HarnessVersion: "2.1.288",
		Source:         harness.SourceLive,
		Scenario:       "deny_shell",
		Event:          "PreToolUse",
		Args:           []string{"--hook-type", "PreToolUse"},
		Expect:         harness.OutcomeDeny,
		Payload: json.RawMessage(`{"hook_event_name":"PreToolUse","cwd":"{{WORK}}",` +
			`"tool_name":"Bash","tool_input":{"command":"touch guarded/x"}}`),
		Response: json.RawMessage(`{"hookSpecificOutput":{"hookEventName":"PreToolUse",` +
			`"permissionDecision":"deny","permissionDecisionReason":"[POL001] no"}}`),
	}
}

var _ = Describe("Fixture", func() {
	It("validates a well-formed live fixture", func() {
		Expect(denyFixture().Validate()).To(Succeed())
	})

	DescribeTable(
		"rejects fixtures that would mislead the contract check",
		func(mutate func(*harness.Fixture), message string) {
			fixture := denyFixture()
			mutate(&fixture)
			Expect(fixture.Validate()).To(MatchError(ContainSubstring(message)))
		},
		Entry("unknown source", func(f *harness.Fixture) { f.Source = "guess" }, "unknown source"),
		Entry("missing version", func(f *harness.Fixture) { f.HarnessVersion = "" }, "required"),
		Entry(
			"live without response",
			func(f *harness.Fixture) { f.Response = nil },
			"record the response",
		),
		Entry(
			"outcome mismatch",
			func(f *harness.Fixture) { f.Expect = harness.OutcomePass },
			"want",
		),
		Entry(
			"bad payload",
			func(f *harness.Fixture) { f.Payload = json.RawMessage(`{}`) },
			"want",
		),
		Entry("bad response", func(f *harness.Fixture) { f.Response = json.RawMessage(`{"x":1}`) },
			"unsupported field"),
		Entry(
			"api key",
			func(f *harness.Fixture) { f.Config = "key = \"sk-harnessfakefakefakefake\"" },
			"leaks",
		),
		Entry(
			"oauth token field",
			func(f *harness.Fixture) { f.Config = `{"access_token": 1}` },
			"leaks",
		),
		Entry(
			"user home path",
			func(f *harness.Fixture) { f.Config = "/Users/someone/.codex" },
			"leaks",
		),
		Entry(
			"temp path",
			func(f *harness.Fixture) { f.Config = "dir = \"/private/var/folders/x\"" },
			"leaks",
		),
		Entry("bare tmp path", func(f *harness.Fixture) { f.Config = "dir = /tmp/run" }, "leaks"),
	)

	It("allows a docs fixture without a recorded response", func() {
		fixture := denyFixture()
		fixture.Source = harness.SourceDocs
		fixture.Response = nil
		Expect(fixture.Validate()).To(Succeed())
	})

	It("does not flag a harness directory that merely ends in tmp", func() {
		fixture := denyFixture()
		fixture.Config = "{{HOME}}/.gemini/tmp/project/chats"
		Expect(fixture.Validate()).To(Succeed())
	})

	It("treats a recorded null response as a pass", func() {
		fixture := denyFixture()
		fixture.Expect = harness.OutcomePass
		Expect(fixture.CheckResponse([]byte("null"))).To(Succeed())
	})

	It("round-trips through WriteFixture and LoadFixtures", func() {
		dir := GinkgoT().TempDir()
		path, err := harness.WriteFixture(dir, denyFixture())
		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(filepath.Join(dir, "claude", "deny_shell-pretooluse.json")))

		loaded, err := harness.LoadFixtures(dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded).To(HaveLen(1))
		Expect(loaded[0].Path()).To(Equal(path))
		Expect(loaded[0].Validate()).To(Succeed())
	})

	It("refuses fixtures with fields it does not know", func() {
		dir := GinkgoT().TempDir()
		Expect(
			os.WriteFile(filepath.Join(dir, "x.json"), []byte(`{"surprise":1}`), 0o600),
		).To(Succeed())

		_, err := harness.LoadFixtures(dir)
		Expect(err).To(MatchError(ContainSubstring("unknown field")))
	})

	It("reports a missing fixture directory", func() {
		_, err := harness.LoadFixtures(filepath.Join(GinkgoT().TempDir(), "absent"))
		Expect(err).To(HaveOccurred())
	})

	It("expands path placeholders", func() {
		Expect(harness.Expand("{{WORK}}/a {{HOME}}/b", "/w", "/h")).To(Equal("/w/a /h/b"))
	})
})
