package updater

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("removeTempFile", func() {
	It("removes existing temporary files", func() {
		tmpDir := GinkgoT().TempDir()
		target := filepath.Join(tmpDir, "target.tmp")

		Expect(os.WriteFile(target, []byte("payload"), 0o600)).To(Succeed())

		removeTempFile(target)

		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("ignores missing files", func() {
		tmpDir := GinkgoT().TempDir()
		target := filepath.Join(tmpDir, "missing.tmp")

		removeTempFile(target)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
