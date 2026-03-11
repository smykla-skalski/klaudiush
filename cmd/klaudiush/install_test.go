package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Install", func() {
	var tempDir string

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "klaudiush-install-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("loadRawSettings", func() {
		It("returns empty map for missing file", func() {
			result, err := loadRawSettings(filepath.Join(tempDir, "nonexistent.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("returns empty map for empty file", func() {
			path := filepath.Join(tempDir, "empty.json")
			err := os.WriteFile(path, []byte(""), 0o600)
			Expect(err).NotTo(HaveOccurred())

			result, err := loadRawSettings(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("parses valid JSON", func() {
			path := filepath.Join(tempDir, "valid.json")
			err := os.WriteFile(
				path,
				[]byte(`{"permissions":{"allow":["Read"]}}`),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := loadRawSettings(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("permissions"))
		})

		It("returns error for invalid JSON", func() {
			path := filepath.Join(tempDir, "invalid.json")
			err := os.WriteFile(path, []byte(`{not json}`), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = loadRawSettings(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse settings"))
		})
	})

	Describe("addHookToSettings", func() {
		It("creates hooks structure when absent", func() {
			raw := map[string]any{}
			addHookToSettings(raw, "/usr/local/bin/klaudiush")

			hooks, ok := raw["hooks"].(map[string]any)
			Expect(ok).To(BeTrue())

			preToolUse, ok := hooks["PreToolUse"].([]any)
			Expect(ok).To(BeTrue())
			Expect(preToolUse).To(HaveLen(1))

			entry, ok := preToolUse[0].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(entry["matcher"]).To(Equal("Bash|Write|Edit"))

			innerHooks, ok := entry["hooks"].([]any)
			Expect(ok).To(BeTrue())
			Expect(innerHooks).To(HaveLen(1))

			cmd, ok := innerHooks[0].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(cmd["type"]).To(Equal("command"))
			Expect(cmd["command"]).To(Equal("/usr/local/bin/klaudiush --hook-type PreToolUse"))
			Expect(cmd["timeout"]).To(Equal(defaultHookTimeout))
		})

		It("appends to existing PreToolUse hooks", func() {
			raw := map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "other-tool",
								},
							},
						},
					},
				},
			}

			addHookToSettings(raw, "/usr/local/bin/klaudiush")

			hooks := raw["hooks"].(map[string]any)
			preToolUse := hooks["PreToolUse"].([]any)
			Expect(preToolUse).To(HaveLen(2))
		})
	})

	Describe("performInstall", func() {
		const fakeBinary = "/usr/local/bin/klaudiush"

		It("creates settings file when it does not exist", func() {
			settingsPath := filepath.Join(tempDir, ".claude", "settings.json")

			err := performClaudeInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())
			Expect(settingsPath).To(BeAnExistingFile())

			data, err := os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any

			err = json.Unmarshal(data, &result)
			Expect(err).NotTo(HaveOccurred())

			hooks, _ := result["hooks"].(map[string]any)
			preToolUse, _ := hooks["PreToolUse"].([]any)
			Expect(preToolUse).To(HaveLen(1))
		})

		It("skips when already registered", func() {
			settingsPath := filepath.Join(tempDir, "settings.json")
			existing := map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash|Write|Edit",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": fakeBinary + " --hook-type PreToolUse",
									"timeout": float64(30),
								},
							},
						},
					},
				},
			}

			data, err := json.MarshalIndent(existing, "", "  ")
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(settingsPath, data, 0o600)
			Expect(err).NotTo(HaveOccurred())

			originalData := make([]byte, len(data))
			copy(originalData, data)

			err = performClaudeInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())

			// File should be unchanged
			afterData, err := os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterData).To(Equal(originalData))
		})

		It("preserves existing settings fields", func() {
			settingsPath := filepath.Join(tempDir, "settings.json")
			existing := map[string]any{
				"permissions": map[string]any{
					"allow": []any{"Read", "Write"},
				},
				"allowedTools": []any{"Bash", "Edit"},
			}

			data, err := json.MarshalIndent(existing, "", "  ")
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(settingsPath, data, 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = performClaudeInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())

			data, err = os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any

			err = json.Unmarshal(data, &result)
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveKey("permissions"))
			Expect(result).To(HaveKey("allowedTools"))
			Expect(result).To(HaveKey("hooks"))
		})

		It("preserves existing PreToolUse hooks", func() {
			settingsPath := filepath.Join(tempDir, "settings.json")
			existing := map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "other-validator --hook-type PreToolUse",
									"timeout": float64(15),
								},
							},
						},
					},
				},
			}

			data, err := json.MarshalIndent(existing, "", "  ")
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(settingsPath, data, 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = performClaudeInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())

			data, err = os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any

			err = json.Unmarshal(data, &result)
			Expect(err).NotTo(HaveOccurred())

			hooks := result["hooks"].(map[string]any)
			preToolUse := hooks["PreToolUse"].([]any)
			Expect(preToolUse).To(HaveLen(2))
		})

		It("returns error for invalid JSON", func() {
			settingsPath := filepath.Join(tempDir, "settings.json")
			err := os.WriteFile(settingsPath, []byte(`{broken`), 0o600)
			Expect(err).NotTo(HaveOccurred())

			err = performClaudeInstall(settingsPath, fakeBinary)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("performCodexInstall", func() {
		const fakeBinary = "/usr/local/bin/klaudiush"

		It("creates hooks.json with SessionStart, AfterToolUse, and Stop hooks", func() {
			hooksPath := filepath.Join(tempDir, ".codex", "hooks.json")

			err := performCodexInstall(hooksPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())
			Expect(hooksPath).To(BeAnExistingFile())

			data, err := os.ReadFile(hooksPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any
			Expect(json.Unmarshal(data, &result)).To(Succeed())

			hooks := result["hooks"].(map[string]any)
			sessionStart := hooks["SessionStart"].([]any)
			afterToolUse := hooks["AfterToolUse"].([]any)
			stop := hooks["Stop"].([]any)

			Expect(sessionStart).To(HaveLen(1))
			Expect(afterToolUse).To(HaveLen(1))
			Expect(stop).To(HaveLen(1))

			sessionStartHooks := sessionStart[0].(map[string]any)["hooks"].([]any)
			afterToolUseHooks := afterToolUse[0].(map[string]any)["hooks"].([]any)
			stopHooks := stop[0].(map[string]any)["hooks"].([]any)

			Expect(sessionStartHooks[0].(map[string]any)["command"]).
				To(Equal(fakeBinary + " --provider codex --event SessionStart"))
			Expect(afterToolUseHooks[0].(map[string]any)["command"]).
				To(Equal(fakeBinary + " --provider codex --event AfterToolUse"))
			Expect(stopHooks[0].(map[string]any)["command"]).
				To(Equal(fakeBinary + " --provider codex --event Stop"))
		})

		It("only adds missing Codex events", func() {
			hooksPath := filepath.Join(tempDir, ".codex", "hooks.json")
			Expect(os.MkdirAll(filepath.Dir(hooksPath), 0o755)).To(Succeed())

			existing := map[string]any{
				"hooks": map[string]any{
					"SessionStart": []any{
						map[string]any{
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": fakeBinary + " --provider codex --event SessionStart",
									"timeout": float64(30),
								},
							},
						},
					},
				},
			}

			data, err := json.MarshalIndent(existing, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(hooksPath, data, 0o600)).To(Succeed())

			err = performCodexInstall(hooksPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())

			data, err = os.ReadFile(hooksPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any
			Expect(json.Unmarshal(data, &result)).To(Succeed())

			hooks := result["hooks"].(map[string]any)
			Expect(hooks["SessionStart"].([]any)).To(HaveLen(1))
			Expect(hooks["AfterToolUse"].([]any)).To(HaveLen(1))
			Expect(hooks["Stop"].([]any)).To(HaveLen(1))
		})

		It("expands tilde paths into the user home directory", func() {
			homeDir := filepath.Join(tempDir, "home")
			Expect(os.MkdirAll(homeDir, 0o755)).To(Succeed())

			oldHome := os.Getenv("HOME")

			Expect(os.Setenv("HOME", homeDir)).To(Succeed())
			DeferCleanup(func() {
				_ = os.Setenv("HOME", oldHome)
			})

			oldWD, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tempDir)).To(Succeed())
			DeferCleanup(func() {
				_ = os.Chdir(oldWD)
			})

			err = performCodexInstall("~/.codex/hooks.json", fakeBinary)
			Expect(err).NotTo(HaveOccurred())
			Expect(filepath.Join(homeDir, ".codex", "hooks.json")).To(BeAnExistingFile())

			_, err = os.Stat(filepath.Join(tempDir, "~"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	Describe("performGeminiInstall", func() {
		const fakeBinary = "/usr/local/bin/klaudiush"

		It("creates settings.json with all supported Gemini hooks", func() {
			settingsPath := filepath.Join(tempDir, ".gemini", "settings.json")

			err := performGeminiInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())
			Expect(settingsPath).To(BeAnExistingFile())

			data, err := os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any
			Expect(json.Unmarshal(data, &result)).To(Succeed())

			hooks := result["hooks"].(map[string]any)
			for _, eventName := range []string{
				"BeforeTool",
				"AfterTool",
				"SessionStart",
				"SessionEnd",
				"Notification",
				"PreCompress",
			} {
				eventHooks := hooks[eventName].([]any)
				Expect(eventHooks).To(HaveLen(1))

				configHooks := eventHooks[0].(map[string]any)["hooks"].([]any)
				Expect(configHooks[0].(map[string]any)["command"]).
					To(Equal(fakeBinary + " --provider gemini --event " + eventName))
			}
		})

		It("only adds missing Gemini events", func() {
			settingsPath := filepath.Join(tempDir, ".gemini", "settings.json")
			Expect(os.MkdirAll(filepath.Dir(settingsPath), 0o755)).To(Succeed())

			existing := map[string]any{
				"hooks": map[string]any{
					"BeforeTool": []any{
						map[string]any{
							"matcher": "run_shell_command|write_file|replace|read_file|glob|grep|ls",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": fakeBinary + " --provider gemini --event BeforeTool",
									"timeout": float64(30000),
								},
							},
						},
					},
				},
			}

			data, err := json.MarshalIndent(existing, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(settingsPath, data, 0o600)).To(Succeed())

			err = performGeminiInstall(settingsPath, fakeBinary)
			Expect(err).NotTo(HaveOccurred())

			data, err = os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any
			Expect(json.Unmarshal(data, &result)).To(Succeed())

			hooks := result["hooks"].(map[string]any)
			Expect(hooks["BeforeTool"].([]any)).To(HaveLen(1))
			Expect(hooks["AfterTool"].([]any)).To(HaveLen(1))
			Expect(hooks["SessionStart"].([]any)).To(HaveLen(1))
			Expect(hooks["SessionEnd"].([]any)).To(HaveLen(1))
			Expect(hooks["Notification"].([]any)).To(HaveLen(1))
			Expect(hooks["PreCompress"].([]any)).To(HaveLen(1))
		})
	})

	Describe("performConfiguredInstall", func() {
		const fakeBinary = "/usr/local/bin/klaudiush"

		It("installs only configured providers", func() {
			claudeSettingsPath := filepath.Join(tempDir, ".claude", "settings.json")
			codexHooksPath := filepath.Join(tempDir, ".codex", "hooks.json")
			geminiSettingsPath := filepath.Join(tempDir, ".gemini", "settings.json")

			claudeEnabled := false
			codexEnabled := true
			codexExperimental := true
			geminiEnabled := true
			cfg := &pkgConfig.Config{
				Providers: &pkgConfig.ProvidersConfig{
					Claude: &pkgConfig.ClaudeProviderConfig{Enabled: &claudeEnabled},
					Codex: &pkgConfig.CodexProviderConfig{
						Enabled:         &codexEnabled,
						Experimental:    &codexExperimental,
						HooksConfigPath: codexHooksPath,
					},
					Gemini: &pkgConfig.GeminiProviderConfig{
						Enabled:      &geminiEnabled,
						SettingsPath: geminiSettingsPath,
					},
				},
			}

			err := performConfiguredInstall(claudeSettingsPath, fakeBinary, cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(codexHooksPath).To(BeAnExistingFile())
			Expect(geminiSettingsPath).To(BeAnExistingFile())
			Expect(claudeSettingsPath).NotTo(BeAnExistingFile())
		})
	})
})
