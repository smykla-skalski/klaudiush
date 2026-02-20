package updater_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/updater"
)

var _ = Describe("ExtractBinaryFromTarGz", func() {
	It("extracts binary at root level", func() {
		archivePath := createTestTarGz("klaudiush", []byte("binary-content"))

		path, cleanup, err := updater.ExtractBinaryFromTarGz(archivePath, "klaudiush")
		Expect(err).NotTo(HaveOccurred())

		defer cleanup()

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("binary-content"))
	})

	It("extracts binary in subdirectory", func() {
		archivePath := createTestTarGzWithPath("dist/klaudiush", []byte("nested-binary"))

		path, cleanup, err := updater.ExtractBinaryFromTarGz(archivePath, "klaudiush")
		Expect(err).NotTo(HaveOccurred())

		defer cleanup()

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("nested-binary"))
	})

	It("returns error when binary not found", func() {
		archivePath := createTestTarGz("other-binary", []byte("content"))

		_, _, err := updater.ExtractBinaryFromTarGz(archivePath, "klaudiush")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("returns error for invalid archive", func() {
		tmpFile := filepath.Join(GinkgoT().TempDir(), "bad.tar.gz")
		Expect(os.WriteFile(tmpFile, []byte("not a tar.gz"), 0o644)).To(Succeed())

		_, _, err := updater.ExtractBinaryFromTarGz(tmpFile, "klaudiush")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ExtractBinaryFromZip", func() {
	It("extracts binary", func() {
		archivePath := createTestZip("klaudiush.exe", []byte("exe-content"))

		path, cleanup, err := updater.ExtractBinaryFromZip(archivePath, "klaudiush")
		Expect(err).NotTo(HaveOccurred())

		defer cleanup()

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("exe-content"))
	})

	It("returns error when binary not found", func() {
		archivePath := createTestZip("other.exe", []byte("content"))

		_, _, err := updater.ExtractBinaryFromZip(archivePath, "klaudiush")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ReplaceBinary", func() {
	It("replaces binary atomically", func() {
		dir := GinkgoT().TempDir()

		target := filepath.Join(dir, "klaudiush")
		Expect(os.WriteFile(target, []byte("old-binary"), 0o755)).To(Succeed())

		newBin := filepath.Join(dir, "new-klaudiush")
		Expect(os.WriteFile(newBin, []byte("new-binary"), 0o755)).To(Succeed())

		Expect(updater.ReplaceBinary(newBin, target)).To(Succeed())

		data, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("new-binary"))

		info, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode()&0o111).NotTo(BeZero(), "replaced binary should be executable")
	})

	It("returns error when target does not exist", func() {
		dir := GinkgoT().TempDir()
		newBin := filepath.Join(dir, "new")
		Expect(os.WriteFile(newBin, []byte("content"), 0o755)).To(Succeed())

		err := updater.ReplaceBinary(newBin, filepath.Join(dir, "nonexistent"))
		Expect(err).To(HaveOccurred())
	})
})

// createTestTarGz creates a tar.gz archive with a single file at root level.
func createTestTarGz(name string, content []byte) string {
	return createTestTarGzWithPath(name, content)
}

// createTestTarGzWithPath creates a tar.gz archive with a file at the given path.
func createTestTarGzWithPath(name string, content []byte) string {
	path := filepath.Join(GinkgoT().TempDir(), "test.tar.gz")

	f, err := os.Create(path)
	Expect(err).NotTo(HaveOccurred())

	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	header := &tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0o755,
	}

	Expect(tw.WriteHeader(header)).To(Succeed())

	_, err = tw.Write(content)
	Expect(err).NotTo(HaveOccurred())

	return path
}

// createTestZip creates a zip archive with a single file.
func createTestZip(name string, content []byte) string {
	path := filepath.Join(GinkgoT().TempDir(), "test.zip")

	f, err := os.Create(path)
	Expect(err).NotTo(HaveOccurred())

	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	w, err := zw.Create(name)
	Expect(err).NotTo(HaveOccurred())

	_, err = w.Write(content)
	Expect(err).NotTo(HaveOccurred())

	return path
}
