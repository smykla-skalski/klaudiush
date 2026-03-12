package xdg

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMigrateInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "XDG Internal Suite")
}

var _ = Describe("copyAndRemove", func() {
	It("copies data to destination and removes source", func() {
		tmpDir := GinkgoT().TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		dstDir := filepath.Join(tmpDir, "dst")

		Expect(os.MkdirAll(srcDir, 0o700)).To(Succeed())
		Expect(os.MkdirAll(dstDir, 0o700)).To(Succeed())

		src := filepath.Join(srcDir, "source.txt")
		dst := filepath.Join(dstDir, "source.txt")

		const data = "xdg migrate"

		Expect(os.WriteFile(src, []byte(data), 0o600)).To(Succeed())

		Expect(copyAndRemove(src, dst)).To(Succeed())

		migrated, err := os.ReadFile(dst)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(migrated)).To(Equal(data))

		_, statErr := os.Stat(src)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})

	It("returns error when destination cannot be written", func() {
		tmpDir := GinkgoT().TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		dstDir := filepath.Join(tmpDir, "readonly")

		Expect(os.MkdirAll(srcDir, 0o700)).To(Succeed())
		Expect(os.MkdirAll(dstDir, 0o500)).To(Succeed())

		src := filepath.Join(srcDir, "source.txt")
		dst := filepath.Join(dstDir, "source.txt")

		const data = "xdg migrate"

		Expect(os.WriteFile(src, []byte(data), 0o600)).To(Succeed())

		err := copyAndRemove(src, dst)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("writing"))
	})
})
