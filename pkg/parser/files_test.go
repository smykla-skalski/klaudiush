package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/parser"
)

var _ = Describe("Files", func() {
	Describe("IsProtectedPath", func() {
		Context("with /tmp paths", func() {
			It("detects exact /tmp", func() {
				Expect(parser.IsProtectedPath("/tmp")).To(BeTrue())
			})

			It("detects /tmp/ prefix", func() {
				Expect(parser.IsProtectedPath("/tmp/file.txt")).To(BeTrue())
				Expect(parser.IsProtectedPath("/tmp/dir/file.txt")).To(BeTrue())
			})

			It("does not match /tmpdir", func() {
				Expect(parser.IsProtectedPath("/tmpdir")).To(BeFalse())
				Expect(parser.IsProtectedPath("/tmpfile")).To(BeFalse())
			})

			It("does not match tmp/ without leading slash", func() {
				Expect(parser.IsProtectedPath("tmp/file.txt")).To(BeFalse())
				Expect(parser.IsProtectedPath("./tmp/file.txt")).To(BeFalse())
			})
		})

		Context("with /var/tmp paths", func() {
			It("detects exact /var/tmp", func() {
				Expect(parser.IsProtectedPath("/var/tmp")).To(BeTrue())
			})

			It("detects /var/tmp/ prefix", func() {
				Expect(parser.IsProtectedPath("/var/tmp/file.txt")).To(BeTrue())
				Expect(parser.IsProtectedPath("/var/tmp/dir/file.txt")).To(BeTrue())
			})

			It("does not match /var/tmpdir", func() {
				Expect(parser.IsProtectedPath("/var/tmpdir")).To(BeFalse())
			})
		})

		Context("with allowed paths", func() {
			It("allows project tmp/", func() {
				Expect(parser.IsProtectedPath("tmp/file.txt")).To(BeFalse())
			})

			It("allows relative paths", func() {
				Expect(parser.IsProtectedPath("./file.txt")).To(BeFalse())
				Expect(parser.IsProtectedPath("../file.txt")).To(BeFalse())
			})

			It("allows home directory", func() {
				Expect(parser.IsProtectedPath("/home/user/file.txt")).To(BeFalse())
			})

			It("allows current directory", func() {
				Expect(parser.IsProtectedPath("file.txt")).To(BeFalse())
				Expect(parser.IsProtectedPath("dir/file.txt")).To(BeFalse())
			})
		})
	})

	Describe("FileWrite", func() {
		It("checks if path is protected", func() {
			fw := parser.FileWrite{
				Path:      "/tmp/output.txt",
				Operation: parser.WriteOpRedirect,
			}
			Expect(fw.IsProtectedPath()).To(BeTrue())

			fw.Path = "tmp/output.txt"
			Expect(fw.IsProtectedPath()).To(BeFalse())
		})

		It("converts to string", func() {
			fw := parser.FileWrite{
				Path:      "/tmp/output.txt",
				Operation: parser.WriteOpTee,
				Source:    "tee",
			}
			Expect(fw.String()).To(ContainSubstring("Tee"))
			Expect(fw.String()).To(ContainSubstring("/tmp/output.txt"))
		})
	})

	Describe("PathValidator", func() {
		var validator *parser.PathValidator

		BeforeEach(func() {
			validator = parser.NewPathValidator()
		})

		It("detects protected path violations", func() {
			writes := []parser.FileWrite{
				{
					Path:      "/tmp/output.txt",
					Operation: parser.WriteOpRedirect,
					Location:  parser.Location{Line: 1, Column: 10},
				},
				{
					Path:      "tmp/safe.txt",
					Operation: parser.WriteOpRedirect,
					Location:  parser.Location{Line: 2, Column: 10},
				},
			}

			violations := validator.CheckProtectedPaths(writes)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Path).To(Equal("/tmp/output.txt"))
			Expect(violations[0].Suggestion).To(ContainSubstring("tmp/"))
		})

		It("returns empty list for safe paths", func() {
			writes := []parser.FileWrite{
				{
					Path:      "tmp/output.txt",
					Operation: parser.WriteOpRedirect,
				},
				{
					Path:      "./file.txt",
					Operation: parser.WriteOpRedirect,
				},
			}

			violations := validator.CheckProtectedPaths(writes)
			Expect(violations).To(BeEmpty())
		})

		It("provides helpful suggestions", func() {
			writes := []parser.FileWrite{
				{
					Path:      "/tmp/test.txt",
					Operation: parser.WriteOpRedirect,
					Location:  parser.Location{Line: 5, Column: 20},
				},
			}

			violations := validator.CheckProtectedPaths(writes)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Suggestion).To(ContainSubstring("mkdir -p tmp/"))
			Expect(violations[0].Suggestion).To(ContainSubstring("tmp/test.txt"))
		})
	})

	Describe("File write detection in commands", func() {
		var p *parser.BashParser

		BeforeEach(func() {
			p = parser.NewBashParser()
		})

		Context("with redirections", func() {
			It("detects output redirection to /tmp", func() {
				result, err := p.Parse("echo 'test' > /tmp/output.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("/tmp/output.txt"))
				Expect(fw.IsProtectedPath()).To(BeTrue())
			})

			It("allows redirection to project tmp/", func() {
				result, err := p.Parse("echo 'test' > tmp/output.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("tmp/output.txt"))
				Expect(fw.IsProtectedPath()).To(BeFalse())
			})
		})

		Context("with tee command", func() {
			It("detects tee to /tmp", func() {
				result, err := p.Parse("echo 'test' | tee /tmp/output.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("/tmp/output.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpTee))
				Expect(fw.IsProtectedPath()).To(BeTrue())
			})

			It("allows tee to project tmp/", func() {
				result, err := p.Parse("echo 'test' | tee tmp/output.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.IsProtectedPath()).To(BeFalse())
			})
		})

		Context("with cp/mv commands", func() {
			It("detects cp to /tmp", func() {
				result, err := p.Parse("cp source.txt /tmp/dest.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("/tmp/dest.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpCopy))
				Expect(fw.IsProtectedPath()).To(BeTrue())
			})

			It("detects mv to /var/tmp", func() {
				result, err := p.Parse("mv old.txt /var/tmp/new.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("/var/tmp/new.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpMove))
				Expect(fw.IsProtectedPath()).To(BeTrue())
			})
		})

		Context("with chained commands", func() {
			It("detects multiple file writes", func() {
				result, err := p.Parse("echo 'a' > /tmp/a.txt && echo 'b' > /tmp/b.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(2))

				Expect(result.FileWrites[0].Path).To(Equal("/tmp/a.txt"))
				Expect(result.FileWrites[0].IsProtectedPath()).To(BeTrue())
				Expect(result.FileWrites[1].Path).To(Equal("/tmp/b.txt"))
				Expect(result.FileWrites[1].IsProtectedPath()).To(BeTrue())
			})
		})
	})
})
