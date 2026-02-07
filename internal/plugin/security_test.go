package plugin_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/plugin"
)

var _ = Describe("Security", func() {
	var (
		tempDir string
		homeDir string
	)

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "plugin-security-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks in temp dir (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())

		homeDir, err = os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("ValidatePath", func() {
		Context("with path traversal attempts", func() {
			DescribeTable("should reject",
				func(path string) {
					err := plugin.ValidatePath(path, nil)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("traversal"))
				},
				Entry("simple traversal", "../secret"),
				Entry("nested traversal", "plugins/../../secret"),
				Entry("multiple traversal", "../../../etc/passwd"),
				Entry("trailing traversal", "plugins/../"),
			)
		})

		Context("with valid paths", func() {
			It("should accept absolute paths", func() {
				pluginPath := filepath.Join(tempDir, "plugin.so")

				err := os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(pluginPath, []string{tempDir})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept paths in allowed directories", func() {
				allowedDir := filepath.Join(tempDir, "allowed")
				err := os.MkdirAll(allowedDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				pluginPath := filepath.Join(allowedDir, "plugin.so")
				err = os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(pluginPath, []string{allowedDir})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept when path equals allowed directory exactly", func() {
				allowedDir := filepath.Join(tempDir, "plugins")
				err := os.MkdirAll(allowedDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				// Path equals allowed directory (no subdirectory or file)
				err = plugin.ValidatePath(allowedDir, []string{allowedDir})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept when no allowed directories specified", func() {
				pluginPath := filepath.Join(tempDir, "plugin.so")

				err := os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(pluginPath, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with directory restrictions", func() {
			It("should reject paths outside allowed directories", func() {
				outsideDir := filepath.Join(tempDir, "outside")
				err := os.MkdirAll(outsideDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				allowedDir := filepath.Join(tempDir, "allowed")
				err = os.MkdirAll(allowedDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				pluginPath := filepath.Join(outsideDir, "plugin.so")
				err = os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(pluginPath, []string{allowedDir})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not in allowed"))
			})

			It("should accept paths in any of multiple allowed directories", func() {
				dir1 := filepath.Join(tempDir, "dir1")
				dir2 := filepath.Join(tempDir, "dir2")

				err := os.MkdirAll(dir1, 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = os.MkdirAll(dir2, 0o755)
				Expect(err).NotTo(HaveOccurred())

				pluginPath := filepath.Join(dir2, "plugin.so")
				err = os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(pluginPath, []string{dir1, dir2})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with symlinks", func() {
			It("should resolve symlinks consistently on both path and allowed dirs", func() {
				realDir := filepath.Join(tempDir, "real")
				err := os.MkdirAll(realDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				realPlugin := filepath.Join(realDir, "plugin.so")
				err = os.WriteFile(realPlugin, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				linkDir := filepath.Join(tempDir, "link")
				err = os.Symlink(realDir, linkDir)
				Expect(err).NotTo(HaveOccurred())

				linkPath := filepath.Join(linkDir, "plugin.so")

				// Both should succeed because symlinks are resolved on BOTH sides
				// This is important for macOS where /var -> /private/var
				err = plugin.ValidatePath(linkPath, []string{linkDir})
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath(linkPath, []string{realDir})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject symlinks pointing outside allowed directories", func() {
				// Create a directory outside the allowed area
				outsideDir := filepath.Join(tempDir, "outside")
				err := os.MkdirAll(outsideDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				outsidePlugin := filepath.Join(outsideDir, "plugin.so")
				err = os.WriteFile(outsidePlugin, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				// Create allowed directory
				allowedDir := filepath.Join(tempDir, "allowed")
				err = os.MkdirAll(allowedDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				// Create symlink inside allowed dir pointing to outside
				linkPath := filepath.Join(allowedDir, "evil-link.so")
				err = os.Symlink(outsidePlugin, linkPath)
				Expect(err).NotTo(HaveOccurred())

				// Should fail - symlink resolves to outside the allowed dir
				err = plugin.ValidatePath(linkPath, []string{allowedDir})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with tilde expansion", func() {
			It("should expand ~ to home directory", func() {
				pluginDir := filepath.Join(homeDir, ".klaudiush", "plugins")
				err := os.MkdirAll(pluginDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				defer os.RemoveAll(filepath.Join(homeDir, ".klaudiush"))

				pluginPath := filepath.Join(pluginDir, "test.so")
				err = os.WriteFile(pluginPath, []byte("test"), 0o755)
				Expect(err).NotTo(HaveOccurred())

				err = plugin.ValidatePath("~/.klaudiush/plugins/test.so", []string{pluginDir})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should reject empty paths", func() {
			err := plugin.ValidatePath("", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("required"))
		})
	})

	Describe("ValidateExtension", func() {
		Context("with allowed extensions", func() {
			DescribeTable("should accept",
				func(path string, allowed []string) {
					err := plugin.ValidateExtension(path, allowed)
					Expect(err).NotTo(HaveOccurred())
				},
				Entry(".so extension", "plugin.so", []string{".so"}),
				Entry("case insensitive", "plugin.SO", []string{".so"}),
				Entry("multiple allowed", "script.sh", []string{".so", ".sh", ".py"}),
			)
		})

		Context("with rejected extensions", func() {
			DescribeTable("should reject",
				func(path string, allowed []string) {
					err := plugin.ValidateExtension(path, allowed)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("extension"))
				},
				Entry("wrong extension", "plugin.dll", []string{".so"}),
				Entry("no extension", "plugin", []string{".so"}),
				Entry("similar but different", "plugin.so.bak", []string{".so"}),
			)
		})

		It("should accept any extension when allowed list is empty", func() {
			err := plugin.ValidateExtension("plugin.anything", nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateMetachars", func() {
		Context("with dangerous characters", func() {
			DescribeTable("should reject",
				func(path string, expectedChar string) {
					err := plugin.ValidateMetachars(path)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("forbidden"))
					Expect(err.Error()).To(ContainSubstring(expectedChar))
				},
				Entry("semicolon", "/path;rm -rf /", ";"),
				Entry("pipe", "/path|cat", "|"),
				Entry("ampersand", "/path&bg", "&"),
				Entry("dollar", "/path$var", "$"),
				Entry("backtick", "/path`cmd`", "`"),
				Entry("double quote", "/path\"quoted\"", "\""),
				Entry("single quote", "/path'quoted'", "'"),
				Entry("less than", "/path<input", "<"),
				Entry("greater than", "/path>output", ">"),
				Entry("open paren", "/path(sub)", "("),
				Entry("close paren", "/path(sub)", "("),
			)
		})

		Context("with safe paths", func() {
			DescribeTable("should accept",
				func(path string) {
					err := plugin.ValidateMetachars(path)
					Expect(err).NotTo(HaveOccurred())
				},
				Entry("simple path", "/home/user/.klaudiush/plugins/plugin.so"),
				Entry("with dashes", "/path/my-plugin-v1.2.3.so"),
				Entry("with underscores", "/path/my_plugin.so"),
				Entry("with dots", "/path/plugin.v1.so"),
				Entry("with numbers", "/path/plugin123.so"),
				Entry("with spaces", "/path/my plugin.so"),
			)
		})
	})

	Describe("GetAllowedDirs", func() {
		It("should return global and project directories", func() {
			dirs, err := plugin.GetAllowedDirs(tempDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(dirs).To(HaveLen(2))

			globalDir := filepath.Join(homeDir, plugin.GlobalPluginDir)
			Expect(dirs).To(ContainElement(globalDir))

			projectDir := filepath.Join(tempDir, plugin.ProjectPluginDir)
			Expect(dirs).To(ContainElement(projectDir))
		})

		It("should return only global directory when projectRoot is empty", func() {
			dirs, err := plugin.GetAllowedDirs("")
			Expect(err).NotTo(HaveOccurred())
			Expect(dirs).To(HaveLen(1))

			globalDir := filepath.Join(homeDir, plugin.GlobalPluginDir)
			Expect(dirs).To(ContainElement(globalDir))
		})
	})

	Describe("IsLocalAddress", func() {
		Context("with localhost addresses", func() {
			DescribeTable("should return true",
				func(address string) {
					Expect(plugin.IsLocalAddress(address)).To(BeTrue())
				},
				Entry("localhost", "localhost"),
				Entry("localhost with port", "localhost:50051"),
				Entry("127.0.0.1", "127.0.0.1"),
				Entry("127.0.0.1 with port", "127.0.0.1:50051"),
				Entry("::1", "::1"),
				Entry("[::1]", "[::1]"),
				Entry("[::1] with port", "[::1]:50051"),
				Entry("0.0.0.0", "0.0.0.0"),
				Entry("0.0.0.0 with port", "0.0.0.0:8080"),
				Entry("LOCALHOST uppercase", "LOCALHOST"),
				Entry("LocalHost mixed case", "LocalHost"),
			)
		})

		Context("with remote addresses", func() {
			DescribeTable("should return false",
				func(address string) {
					Expect(plugin.IsLocalAddress(address)).To(BeFalse())
				},
				Entry("remote IP", "192.168.1.100"),
				Entry("remote IP with port", "192.168.1.100:50051"),
				Entry("domain name", "plugin.example.com"),
				Entry("domain with port", "plugin.example.com:50051"),
				Entry("external IP", "8.8.8.8:443"),
				Entry("IPv6 remote", "2001:db8::1"),
				Entry("empty string", ""),
			)
		})
	})

	Describe("SanitizePanicMessage", func() {
		It("should remove file paths", func() {
			msg := "panic at /home/user/project/plugin.go:42: nil pointer"
			sanitized := plugin.SanitizePanicMessage(msg)
			Expect(sanitized).NotTo(ContainSubstring("/home"))
			Expect(sanitized).To(ContainSubstring("[path]"))
			Expect(sanitized).To(ContainSubstring("nil pointer"))
		})

		It("should truncate long messages", func() {
			longMsg := strings.Repeat("a", 500)
			sanitized := plugin.SanitizePanicMessage(longMsg)
			Expect(len(sanitized)).To(BeNumerically("<=", 203)) // 200 + "..."
			Expect(sanitized).To(HaveSuffix("..."))
		})

		It("should handle empty messages", func() {
			sanitized := plugin.SanitizePanicMessage("")
			Expect(sanitized).To(Equal(""))
		})

		It("should preserve messages without paths", func() {
			msg := "simple panic message"
			sanitized := plugin.SanitizePanicMessage(msg)
			Expect(sanitized).To(Equal(msg))
		})

		It("should handle multiple paths", func() {
			msg := "error at /path/one.go and /path/two.go"
			sanitized := plugin.SanitizePanicMessage(msg)
			Expect(strings.Count(sanitized, "[path]")).To(Equal(2))
		})
	})

	Describe("Sentinel Errors", func() {
		It("should have distinct error values", func() {
			errors := []error{
				plugin.ErrPathTraversal,
				plugin.ErrPathNotAllowed,
				plugin.ErrInvalidExtension,
				plugin.ErrDangerousChars,
				plugin.ErrLoaderClosed,
				plugin.ErrInsecureRemote,
			}

			for i, err1 := range errors {
				for j, err2 := range errors {
					if i != j {
						Expect(err1).NotTo(Equal(err2))
					}
				}
			}
		})
	})
})
