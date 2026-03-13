package settings_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

var _ = Describe("copyFile", func() {
	It("copies file contents to destination", func() {
		tmpDir := GinkgoT().TempDir()
		src := filepath.Join(tmpDir, "source.json")
		dst := filepath.Join(tmpDir, "destination.json")

		const contents = "from settings"

		err := os.WriteFile(src, []byte(contents), 0o600)
		Expect(err).ToNot(HaveOccurred())

		err = settings.CopyFileForTest(src, dst)
		Expect(err).ToNot(HaveOccurred())

		copied, err := os.ReadFile(dst)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(copied)).To(Equal(contents))
	})

	It("returns error when source file cannot be read", func() {
		tmpDir := GinkgoT().TempDir()
		src := filepath.Join(tmpDir, "missing.json")
		dst := filepath.Join(tmpDir, "destination.json")

		err := settings.CopyFileForTest(src, dst)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read source file"))
	})

	It("returns error when destination cannot be written", func() {
		tmpDir := GinkgoT().TempDir()
		src := filepath.Join(tmpDir, "source.json")
		dstDir := filepath.Join(tmpDir, "readonly")
		dst := filepath.Join(dstDir, "destination.json")

		const contents = "from settings"

		err := os.WriteFile(src, []byte(contents), 0o600)
		Expect(err).ToNot(HaveOccurred())

		Expect(os.MkdirAll(dstDir, 0o500)).To(Succeed())

		err = settings.CopyFileForTest(src, dst)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to write destination file"))
	})
})
