package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

			err := performInstall(settingsPath, fakeBinary)
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

			err = performInstall(settingsPath, fakeBinary)
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

			err = performInstall(settingsPath, fakeBinary)
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

			err = performInstall(settingsPath, fakeBinary)
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

			err = performInstall(settingsPath, fakeBinary)
			Expect(err).To(HaveOccurred())
		})
	})
})
