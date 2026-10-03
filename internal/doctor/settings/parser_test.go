package settings_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

func TestSettings(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Settings Parser Suite")
}

var _ = Describe("SettingsParser", func() {
	var testdataDir string

	BeforeEach(func() {
		wd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		testdataDir = filepath.Join(wd, "..", "..", "..", "testdata", "settings")
	})

	Describe("Parse", func() {
		Context("when the settings file is valid with dispatcher", func() {
			It("should parse successfully", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_with_dispatcher.json"),
				)

				result, err := parser.Parse()
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Hooks).To(HaveKey("PreToolUse"))
				Expect(result.Hooks).To(HaveKey("PostToolUse"))
				Expect(result.Hooks["PreToolUse"]).To(HaveLen(1))
				Expect(result.Hooks["PreToolUse"][0].Matcher).To(Equal("Bash|Write|Edit|MultiEdit"))
				Expect(result.Hooks["PreToolUse"][0].Hooks).To(HaveLen(1))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Type).To(Equal("command"))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Command).
					To(Equal("klaudiush --hook-type PreToolUse"))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Timeout).To(Equal(30))
				Expect(result.Hooks["PostToolUse"][0].Hooks[0].Command).
					To(Equal("klaudiush --hook-type PostToolUse"))
			})
		})

		Context("when the settings file is valid without dispatcher", func() {
			It("should parse successfully", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_without_dispatcher.json"),
				)

				result, err := parser.Parse()
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Hooks).To(HaveKey("PreToolUse"))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Command).To(Equal("some-other-tool"))
			})
		})

		Context("when the settings file is empty", func() {
			It("should parse successfully with empty hooks", func() {
				parser := settings.NewSettingsParser(filepath.Join(testdataDir, "empty.json"))

				result, err := parser.Parse()
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Hooks).To(BeEmpty())
			})
		})

		Context("when the settings file has invalid JSON syntax", func() {
			It("should return an error", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "invalid_syntax.json"),
				)

				result, err := parser.Parse()
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, settings.ErrInvalidJSON)).To(BeTrue())
				Expect(result).To(BeNil())
			})
		})

		Context("when the settings file does not exist", func() {
			It("should return ErrSettingsNotFound", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "nonexistent.json"),
				)

				result, err := parser.Parse()
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, settings.ErrSettingsNotFound)).To(BeTrue())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("IsDispatcherRegistered", func() {
		Context("when dispatcher is registered", func() {
			It("should return true", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_with_dispatcher.json"),
				)

				registered, err := parser.IsDispatcherRegistered("klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(registered).To(BeTrue())
			})

			It("should find dispatcher with full path", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_with_dispatcher.json"),
				)

				registered, err := parser.IsDispatcherRegistered(
					"/usr/local/bin/klaudiush",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(registered).To(BeTrue())
			})
		})

		Context("when dispatcher is not registered", func() {
			It("should return false", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_without_dispatcher.json"),
				)

				registered, err := parser.IsDispatcherRegistered("klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(registered).To(BeFalse())
			})
		})

		Context("when settings file does not exist", func() {
			It("should return false without error", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "nonexistent.json"),
				)

				registered, err := parser.IsDispatcherRegistered("klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(registered).To(BeFalse())
			})
		})

		Context("when settings file has invalid JSON", func() {
			It("should return an error", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "invalid_syntax.json"),
				)

				registered, err := parser.IsDispatcherRegistered("klaudiush")
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, settings.ErrInvalidJSON)).To(BeTrue())
				Expect(registered).To(BeFalse())
			})
		})

		Context("when settings file is empty", func() {
			It("should return false", func() {
				parser := settings.NewSettingsParser(filepath.Join(testdataDir, "empty.json"))

				registered, err := parser.IsDispatcherRegistered("klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(registered).To(BeFalse())
			})
		})
	})

	Describe("HasPreToolUseHook", func() {
		Context("when PreToolUse hook exists", func() {
			It("should return true", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_with_dispatcher.json"),
				)

				hasHook, err := parser.HasPreToolUseHook()
				Expect(err).NotTo(HaveOccurred())
				Expect(hasHook).To(BeTrue())
			})
		})

		Context("when PreToolUse hook does not exist", func() {
			It("should return false", func() {
				parser := settings.NewSettingsParser(filepath.Join(testdataDir, "empty.json"))

				hasHook, err := parser.HasPreToolUseHook()
				Expect(err).NotTo(HaveOccurred())
				Expect(hasHook).To(BeFalse())
			})
		})

		Context("when settings file does not exist", func() {
			It("should return false without error", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "nonexistent.json"),
				)

				hasHook, err := parser.HasPreToolUseHook()
				Expect(err).NotTo(HaveOccurred())
				Expect(hasHook).To(BeFalse())
			})
		})

		Context("when settings file has invalid JSON", func() {
			It("should return an error", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "invalid_syntax.json"),
				)

				hasHook, err := parser.HasPreToolUseHook()
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, settings.ErrInvalidJSON)).To(BeTrue())
				Expect(hasHook).To(BeFalse())
			})
		})
	})

	Describe("HasPostToolUseHook", func() {
		Context("when PostToolUse hook exists", func() {
			It("should return true", func() {
				parser := settings.NewSettingsParser(
					filepath.Join(testdataDir, "valid_with_dispatcher.json"),
				)

				hasHook, err := parser.HasPostToolUseHook()
				Expect(err).NotTo(HaveOccurred())
				Expect(hasHook).To(BeTrue())
			})
		})

		Context("when PostToolUse hook does not exist", func() {
			It("should return false", func() {
				parser := settings.NewSettingsParser(filepath.Join(testdataDir, "empty.json"))

				hasHook, err := parser.HasPostToolUseHook()
				Expect(err).NotTo(HaveOccurred())
				Expect(hasHook).To(BeFalse())
			})
		})
	})

	Describe("HasEventHookCommand", func() {
		It("matches a dispatcher command for a specific event", func() {
			parser := settings.NewSettingsParser(
				filepath.Join(testdataDir, "valid_with_dispatcher.json"),
			)

			hasHook, err := parser.HasEventHookCommand("PostToolUse", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeTrue())
		})

		It("does not match an unrelated dispatcher command", func() {
			parser := settings.NewSettingsParser(
				filepath.Join(testdataDir, "valid_without_dispatcher.json"),
			)

			hasHook, err := parser.HasEventHookCommand("PreToolUse", "klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeFalse())
		})
	})

	Describe("Path Functions", func() {
		Describe("GetUserSettingsPath", func() {
			It("should return user settings path", func() {
				path := settings.GetUserSettingsPath()
				Expect(path).NotTo(BeEmpty())
				Expect(path).To(ContainSubstring(".claude"))
				Expect(path).To(HaveSuffix("settings.json"))
			})
		})

		Describe("GetProjectSettingsPath", func() {
			It("should return project settings path", func() {
				path := settings.GetProjectSettingsPath()
				Expect(path).To(Equal(filepath.Join(".claude", "settings.json")))
			})
		})

		Describe("GetProjectLocalSettingsPath", func() {
			It("should return project-local settings path", func() {
				path := settings.GetProjectLocalSettingsPath()
				Expect(path).To(Equal(filepath.Join(".claude", "settings.local.json")))
			})
		})

		Describe("GetEnterprisePolicyPaths", func() {
			It("should return platform-specific paths", func() {
				paths := settings.GetEnterprisePolicyPaths()
				// On macOS or Linux, should have at least one path
				if os.Getenv("GOOS") == "darwin" || os.Getenv("GOOS") == "linux" {
					Expect(paths).NotTo(BeEmpty())
				}
			})
		})

		Describe("GetAllSettingsPaths", func() {
			It("should return all possible settings locations", func() {
				locations := settings.GetAllSettingsPaths()
				Expect(len(locations)).To(BeNumerically(">=", 3))

				var foundUser, foundProject, foundProjectLocal bool

				for _, loc := range locations {
					switch loc.Type {
					case "user":
						foundUser = true

						Expect(loc.Path).To(ContainSubstring(".claude"))
					case "project":
						foundProject = true

						Expect(loc.Path).To(Equal(filepath.Join(".claude", "settings.json")))
					case "project-local":
						foundProjectLocal = true

						Expect(loc.Path).To(Equal(
							filepath.Join(".claude", "settings.local.json"),
						))
					case "enterprise":
						Expect(loc.Path).NotTo(BeEmpty())
					}
				}

				Expect(foundUser).To(BeTrue())
				Expect(foundProject).To(BeTrue())
				Expect(foundProjectLocal).To(BeTrue())
			})

			It("should check file existence", func() {
				locations := settings.GetAllSettingsPaths()
				for _, loc := range locations {
					if loc.Path != "" {
						_, statErr := os.Stat(loc.Path)
						if statErr == nil {
							Expect(loc.Exists).To(BeTrue())
						} else {
							Expect(loc.Exists).To(BeFalse())
						}
					}
				}
			})
		})
	})

	Describe("CodexHooksParser", func() {
		var hooksPath string

		BeforeEach(func() {
			tempDir, err := os.MkdirTemp("", "codex-hooks-parser-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.RemoveAll(tempDir) })

			hooksPath = filepath.Join(tempDir, "hooks.json")
		})

		It("parses valid Codex hooks files", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart","timeout":30}]}],
    "PreToolUse": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","timeout":30}]}],
    "Stop": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop","timeout":30}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewCodexHooksParser(hooksPath)
			result, err := parser.Parse()

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Hooks.SessionStart).To(HaveLen(1))
			Expect(result.Hooks.PreToolUse).To(HaveLen(1))
			Expect(result.Hooks.Stop).To(HaveLen(1))
			Expect(result.Hooks.SessionStart[0].Hooks[0].Command).
				To(Equal("klaudiush --provider codex --event SessionStart"))
			Expect(result.Hooks.PreToolUse[0].Hooks[0].Command).
				To(Equal("klaudiush --provider codex --event PreToolUse"))
		})

		DescribeTable("tells a dispatcher run from a lookup of it",
			func(command string, want bool) {
				raw := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":` +
					strconv.Quote(command) + `}]}]}}`
				Expect(os.WriteFile(hooksPath, []byte(raw), 0o600)).To(Succeed())

				found, err := settings.NewCodexHooksParser(hooksPath).
					HasEventHook("PreToolUse", "/usr/local/bin/klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(Equal(want))
			},
			Entry("command wrapper", "command klaudiush --provider codex", true),
			Entry("command -- wrapper", "command -- klaudiush --provider codex", true),
			Entry("command -v", "command -v klaudiush", false),
			Entry("command -V", "command -V klaudiush", false),
		)

		It("finds event-specific dispatcher hooks", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart","timeout":30}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewCodexHooksParser(hooksPath)
			hasSessionStart, err := parser.HasEventHook("SessionStart", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasSessionStart).To(BeTrue())

			hasStop, err := parser.HasEventHook("Stop", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasStop).To(BeFalse())
		})

		It("finds PreToolUse hooks by canonical alias", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "PreToolUse": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","timeout":30}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewCodexHooksParser(hooksPath)
			hasHook, err := parser.HasEventHook("before_tool", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeTrue())
		})

		It("does not treat legacy AfterToolUse as PostToolUse", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "AfterToolUse": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event AfterToolUse","timeout":30}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewCodexHooksParser(hooksPath)
			hasPostTool, err := parser.HasEventHook("after_tool", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasPostTool).To(BeFalse())

			enforcement, err := parser.PreToolEnforcement("/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(enforcement.Registered).To(BeFalse())
			Expect(enforcement.LegacyOnly).To(BeTrue())
		})

		It("does not mistake a wrapper script named after klaudiush for the dispatcher", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "PreToolUse": [
      {"hooks":[{"type":"command","command":"/home/u/bin/klaudiush-audit.sh --provider codex"}]},
      {"hooks":[{"type":"command","command":"logger -t klaudiush pre-tool-seen"}]}
    ]
  }
}`),
				0o600,
			)).To(Succeed())

			hasHook, err := settings.NewCodexHooksParser(hooksPath).
				HasEventHook("PreToolUse", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeFalse())
		})

		It("matches the dispatcher behind env assignments", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "PreToolUse": [{"hooks":[{
      "type": "command",
      "command": "env KLAUDIUSH_DEBUG=1 /opt/bin/klaudiush --provider codex --event PreToolUse"
    }]}]
  }
}`),
				0o600,
			)).To(Succeed())

			hasHook, err := settings.NewCodexHooksParser(hooksPath).
				HasEventHook("PreToolUse", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeTrue())
		})

		It("ignores async PreToolUse handlers for enforcement", func() {
			Expect(os.WriteFile(
				hooksPath,
				[]byte(`{
  "hooks": {
    "PreToolUse": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","async":true}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			enforcement, err := settings.NewCodexHooksParser(hooksPath).
				PreToolEnforcement("/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(enforcement.Registered).To(BeTrue())
			Expect(enforcement.AsyncOnly).To(BeTrue())
			Expect(enforcement.EffectiveMatcher).To(BeEmpty())
		})

		DescribeTable(
			"matcher selection",
			func(matcher string, toolNames []string, expected bool) {
				Expect(settings.CodexMatcherSelects(matcher, toolNames)).To(Equal(expected))
			},
			Entry("empty matcher selects everything", "", []string{"Bash"}, true),
			Entry("star selects everything", "*", []string{"mcp__x__y"}, true),
			Entry("alternation selects Bash", "Bash|apply_patch", []string{"Bash"}, true),
			Entry(
				"Edit alias selects apply_patch",
				"Edit|Write",
				[]string{"apply_patch", "Edit"},
				true,
			),
			Entry("Bash-only skips MCP", "Bash", []string{"mcp__x__y"}, false),
			Entry(
				"anchored: Bash does not select BashOutput",
				"Bash",
				[]string{"BashOutput"},
				false,
			),
			Entry("MCP prefix selects MCP", "mcp__.*", []string{"mcp__x__y"}, true),
			Entry("invalid regex selects nothing", "(", []string{"Bash"}, false),
		)

		DescribeTable("SelectsEveryTool needs a sync match-all handler",
			func(hooksJSON string, expected bool) {
				Expect(os.WriteFile(hooksPath, []byte(hooksJSON), 0o600)).To(Succeed())

				enforcement, err := settings.NewCodexHooksParser(hooksPath).
					PreToolEnforcement("/usr/local/bin/klaudiush")
				Expect(err).NotTo(HaveOccurred())
				Expect(enforcement.SelectsEveryTool()).To(Equal(expected))
			},
			Entry(
				"matcherless",
				`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}]}}`,
				true,
			),
			Entry(
				"star matcher",
				`{"hooks":{"PreToolUse":[{"matcher":" * ","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}]}}`,
				true,
			),
			Entry(
				"restrictive matcher",
				`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}]}}`,
				false,
			),
			Entry(
				"async matcherless",
				`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","async":true}]}]}}`,
				false,
			),
			Entry("empty file", ``, false),
		)

		DescribeTable("InstallCodexDispatcher adds a sync matcherless PreToolUse handler",
			func(preToolUse string, wantGroups int) {
				const binary = "/usr/local/bin/klaudiush"

				Expect(os.WriteFile(hooksPath, []byte(`{"hooks":{
  "SessionStart":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart"}]}],
  "Stop":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop"}]}],
  "PreToolUse":[`+preToolUse+`]
}}`), 0o600)).To(Succeed())

				unchanged, err := settings.InstallCodexDispatcher(hooksPath, binary)
				Expect(err).NotTo(HaveOccurred())
				Expect(unchanged).To(BeFalse())

				parser := settings.NewCodexHooksParser(hooksPath)
				enforcement, err := parser.PreToolEnforcement(binary)
				Expect(err).NotTo(HaveOccurred())
				Expect(enforcement.SelectsEveryTool()).To(BeTrue())

				parsed, err := parser.Parse()
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed.Hooks.PreToolUse).To(HaveLen(wantGroups))
				Expect(parsed.Hooks.SessionStart).To(HaveLen(1))
				Expect(parsed.Hooks.Stop).To(HaveLen(1))

				added := parsed.Hooks.PreToolUse[wantGroups-1]
				Expect(added.Matcher).To(BeEmpty())
				Expect(added.Hooks).To(HaveLen(1))
				Expect(added.Hooks[0].Async).To(BeFalse())
				Expect(added.Hooks[0].Command).
					To(Equal(binary + " --provider codex --event PreToolUse"))

				written, err := os.ReadFile(hooksPath)
				Expect(err).NotTo(HaveOccurred())

				unchanged, err = settings.InstallCodexDispatcher(hooksPath, binary)
				Expect(err).NotTo(HaveOccurred())
				Expect(unchanged).To(BeTrue())

				again, err := os.ReadFile(hooksPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(again).To(Equal(written))
			},
			Entry(
				"async only",
				`{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","async":true}]}`,
				2,
			),
			Entry(
				"restrictive matcher only",
				`{"matcher":"Bash","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}`,
				2,
			),
			Entry("no PreToolUse handler", ``, 1),
		)

		It("leaves a complete Codex registration untouched", func() {
			const binary = "/usr/local/bin/klaudiush"

			content := []byte(`{"hooks":{
  "SessionStart":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart"}]}],
  "Stop":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop"}]}],
  "PreToolUse":[
    {"matcher":"Bash","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]},
    {"matcher":"*","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}
  ]
}}`)
			Expect(os.WriteFile(hooksPath, content, 0o600)).To(Succeed())

			unchanged, err := settings.InstallCodexDispatcher(hooksPath, binary)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged).To(BeTrue())

			after, err := os.ReadFile(hooksPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(content))
		})

		It("rejects malformed hooks.json in enforcement and install", func() {
			Expect(os.WriteFile(hooksPath, []byte(`{"hooks":`), 0o600)).To(Succeed())

			_, err := settings.NewCodexHooksParser(hooksPath).
				PreToolEnforcement("/usr/local/bin/klaudiush")
			Expect(err).To(HaveOccurred())

			_, err = settings.InstallCodexDispatcher(hooksPath, "/usr/local/bin/klaudiush")
			Expect(err).To(HaveOccurred())
		})

		It("only counts command handlers whose program is the dispatcher", func() {
			Expect(os.WriteFile(hooksPath, []byte(`{"hooks":{"PreToolUse":[
  {"hooks":[{"type":"prompt","command":"klaudiush --provider codex --event PreToolUse"}]},
  {"hooks":[{"type":"command","command":"FOO=1"}]}
]}}`), 0o600)).To(Succeed())

			enforcement, err := settings.NewCodexHooksParser(hooksPath).
				PreToolEnforcement("/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(enforcement.Registered).To(BeFalse())
		})

		It("fails on an unparsable Codex config.toml", func() {
			Expect(os.WriteFile(
				filepath.Join(filepath.Dir(hooksPath), "config.toml"),
				[]byte("[features\n"),
				0o600,
			)).To(Succeed())

			_, _, err := settings.CodexHooksFeatureDisabled(hooksPath)
			Expect(err).To(MatchError(ContainSubstring("parse Codex config")))
		})

		It("fails when config.toml cannot be read", func() {
			Expect(os.Mkdir(filepath.Join(filepath.Dir(hooksPath), "config.toml"), 0o755)).
				To(Succeed())

			_, _, err := settings.CodexHooksFeatureDisabled(hooksPath)
			Expect(err).To(MatchError(ContainSubstring("read Codex config")))
		})

		Describe("RemoveCodexLegacyDispatcherHooks", func() {
			const binary = "/usr/local/bin/klaudiush"

			dispatcher := func() map[string]any {
				return map[string]any{
					"type":    "command",
					"command": binary + " --provider codex --event AfterToolUse",
				}
			}

			It("ignores configs without hooks or AfterToolUse groups", func() {
				raw := map[string]any{"other": true}
				settings.RemoveCodexLegacyDispatcherHooks(raw, binary)
				Expect(raw).To(Equal(map[string]any{"other": true}))

				raw = map[string]any{"hooks": map[string]any{"AfterToolUse": "bad"}}
				settings.RemoveCodexLegacyDispatcherHooks(raw, binary)
				Expect(raw["hooks"]).To(HaveKeyWithValue("AfterToolUse", "bad"))
			})

			It("keeps malformed groups and unrelated handlers", func() {
				raw := map[string]any{"hooks": map[string]any{"AfterToolUse": []any{
					"not-a-group",
					map[string]any{"matcher": "x"},
					map[string]any{"hooks": []any{"not-a-handler", dispatcher()}},
					map[string]any{"hooks": []any{dispatcher()}},
				}}}

				settings.RemoveCodexLegacyDispatcherHooks(raw, binary)

				kept := raw["hooks"].(map[string]any)["AfterToolUse"].([]any)
				Expect(kept).To(HaveLen(3))
				Expect(kept[0]).To(Equal("not-a-group"))
				Expect(kept[1]).To(Equal(map[string]any{"matcher": "x"}))
				Expect(kept[2].(map[string]any)["hooks"]).To(Equal([]any{"not-a-handler"}))
			})

			It("drops the event once only dispatcher handlers were there", func() {
				raw := map[string]any{"hooks": map[string]any{"AfterToolUse": []any{
					map[string]any{"hooks": []any{dispatcher()}},
				}}}

				settings.RemoveCodexLegacyDispatcherHooks(raw, binary)

				Expect(raw["hooks"]).NotTo(HaveKey("AfterToolUse"))
			})
		})

		It("reports an explicitly disabled hooks feature", func() {
			Expect(os.WriteFile(
				filepath.Join(filepath.Dir(hooksPath), "config.toml"),
				[]byte("model = \"x\"\n\n[features]\nhooks = false\n"),
				0o600,
			)).To(Succeed())

			disabled, configPath, err := settings.CodexHooksFeatureDisabled(hooksPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(disabled).To(BeTrue())
			Expect(configPath).To(HaveSuffix("config.toml"))
		})

		It("treats a missing hooks feature key as enabled", func() {
			Expect(os.WriteFile(
				filepath.Join(filepath.Dir(hooksPath), "config.toml"),
				[]byte("[features]\nother = true\n"),
				0o600,
			)).To(Succeed())

			disabled, _, err := settings.CodexHooksFeatureDisabled(hooksPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(disabled).To(BeFalse())
		})

		It("expands tilde paths before reading hooks.json", func() {
			homeDir := filepath.Join(filepath.Dir(hooksPath), "home")
			Expect(os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755)).To(Succeed())

			oldHome := os.Getenv("HOME")

			Expect(os.Setenv("HOME", homeDir)).To(Succeed())
			DeferCleanup(func() {
				_ = os.Setenv("HOME", oldHome)
			})

			Expect(os.WriteFile(
				filepath.Join(homeDir, ".codex", "hooks.json"),
				[]byte(`{
  "hooks": {
    "Stop": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop","timeout":30}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewCodexHooksParser("~/.codex/hooks.json")
			result, err := parser.Parse()

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Hooks.Stop).To(HaveLen(1))
		})
	})

	Describe("GeminiSettingsParser", func() {
		var settingsPath string

		BeforeEach(func() {
			tempDir, err := os.MkdirTemp("", "gemini-settings-parser-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.RemoveAll(tempDir) })

			settingsPath = filepath.Join(tempDir, "settings.json")
		})

		It("parses valid Gemini settings files", func() {
			Expect(os.WriteFile(
				settingsPath,
				[]byte(`{
  "hooks": {
    "BeforeTool": [{"matcher":"run_shell_command|write_file|replace|read_file|glob|grep|ls","hooks":[{"type":"command","command":"klaudiush --provider gemini --event BeforeTool","timeout":30000}]}],
    "AfterTool": [{"matcher":"run_shell_command|write_file|replace|read_file|glob|grep|ls","hooks":[{"type":"command","command":"klaudiush --provider gemini --event AfterTool","timeout":30000}]}],
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event SessionStart","timeout":30000}]}],
    "SessionEnd": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event SessionEnd","timeout":30000}]}],
    "Notification": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event Notification","timeout":30000}]}],
    "PreCompress": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event PreCompress","timeout":30000}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewGeminiSettingsParser(settingsPath)
			result, err := parser.Parse()

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Hooks.BeforeTool).To(HaveLen(1))
			Expect(result.Hooks.AfterTool).To(HaveLen(1))
			Expect(result.Hooks.SessionStart).To(HaveLen(1))
			Expect(result.Hooks.SessionEnd).To(HaveLen(1))
			Expect(result.Hooks.Notification).To(HaveLen(1))
			Expect(result.Hooks.PreCompress).To(HaveLen(1))
			Expect(result.Hooks.BeforeTool[0].Hooks[0].Command).
				To(Equal("klaudiush --provider gemini --event BeforeTool"))
		})

		It("finds Gemini hooks by event alias", func() {
			Expect(os.WriteFile(
				settingsPath,
				[]byte(`{
  "hooks": {
    "PreCompress": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event PreCompress","timeout":30000}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewGeminiSettingsParser(settingsPath)
			hasHook, err := parser.HasEventHook("pre_compress", "/usr/local/bin/klaudiush")
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeTrue())
		})

		It("registers the AfterAgent completion gate and stays idempotent", func() {
			const binary = "/usr/local/bin/klaudiush"

			Expect(os.WriteFile(settingsPath, []byte(`{"hooks":{
  "SessionEnd": [{"hooks":[{"type":"command","command":"/usr/local/bin/klaudiush --provider gemini --event SessionEnd","timeout":30000}]}]
}}`), 0o600)).To(Succeed())

			unchanged, err := settings.InstallGeminiDispatcher(settingsPath, binary)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged).To(BeFalse())

			parser := settings.NewGeminiSettingsParser(settingsPath)
			result, err := parser.Parse()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Hooks.AfterAgent).To(HaveLen(1))
			Expect(result.Hooks.AfterAgent[0].Matcher).To(BeEmpty())
			Expect(result.Hooks.AfterAgent[0].Hooks[0].Command).
				To(Equal(settings.GeminiAfterAgentCommand(binary)))
			Expect(result.Hooks.SessionEnd).To(HaveLen(1))

			for _, alias := range []string{"AfterAgent", "turn_stop", "SessionEnd", "session_end"} {
				hasHook, hookErr := parser.HasEventHook(alias, binary)
				Expect(hookErr).NotTo(HaveOccurred())
				Expect(hasHook).To(BeTrue(), alias)
			}

			unchanged, err = settings.InstallGeminiDispatcher(settingsPath, binary)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged).To(BeTrue())
			Expect(settings.GeminiDispatcherEvents()).To(ContainElement("AfterAgent"))
		})

		It("expands tilde paths before reading settings.json", func() {
			homeDir := filepath.Join(filepath.Dir(settingsPath), "home")
			Expect(os.MkdirAll(filepath.Join(homeDir, ".gemini"), 0o755)).To(Succeed())

			oldHome := os.Getenv("HOME")

			Expect(os.Setenv("HOME", homeDir)).To(Succeed())
			DeferCleanup(func() {
				_ = os.Setenv("HOME", oldHome)
			})

			Expect(os.WriteFile(
				filepath.Join(homeDir, ".gemini", "settings.json"),
				[]byte(`{
  "hooks": {
    "SessionEnd": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event SessionEnd","timeout":30000}]}]
  }
}`),
				0o600,
			)).To(Succeed())

			parser := settings.NewGeminiSettingsParser("~/.gemini/settings.json")
			result, err := parser.Parse()

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Hooks.SessionEnd).To(HaveLen(1))
		})
	})
})
