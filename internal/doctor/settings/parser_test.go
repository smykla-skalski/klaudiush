package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/doctor/settings"
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
				Expect(result.Hooks["PreToolUse"]).To(HaveLen(1))
				Expect(result.Hooks["PreToolUse"][0].Matcher).To(Equal("Bash|Write|Edit"))
				Expect(result.Hooks["PreToolUse"][0].Hooks).To(HaveLen(1))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Type).To(Equal("command"))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Command).
					To(Equal("klaudiush --hook-type PreToolUse"))
				Expect(result.Hooks["PreToolUse"][0].Hooks[0].Timeout).To(Equal(30))
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
})
