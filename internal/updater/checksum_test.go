package updater_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/updater"
)

var _ = Describe("ParseChecksums", func() {
	DescribeTable("parsing checksum content",
		func(content string, expected map[string]string) {
			got := updater.ParseChecksums(content)
			Expect(got).To(HaveLen(len(expected)))

			for key, wantVal := range expected {
				Expect(got).To(HaveKeyWithValue(key, wantVal))
			}
		},
		Entry("standard checksums.txt",
			"abc123def456  klaudiush_1.13.0_darwin_arm64.tar.gz\n"+
				"789012fed345  klaudiush_1.13.0_linux_amd64.tar.gz\n"+
				"deadbeef0000  klaudiush_1.13.0_windows_amd64.zip",
			map[string]string{
				"klaudiush_1.13.0_darwin_arm64.tar.gz": "abc123def456",
				"klaudiush_1.13.0_linux_amd64.tar.gz":  "789012fed345",
				"klaudiush_1.13.0_windows_amd64.zip":   "deadbeef0000",
			},
		),
		Entry("empty content", "", map[string]string{}),
		Entry("whitespace only", "  \n  \n  ", map[string]string{}),
		Entry("single space separator is ignored",
			"abc123 filename.tar.gz",
			map[string]string{},
		),
		Entry("trailing newline",
			"abc123  file.tar.gz\n",
			map[string]string{"file.tar.gz": "abc123"},
		),
	)
})

var _ = Describe("VerifyFileChecksum", func() {
	var (
		tmpFile     string
		expectedHex string
	)

	BeforeEach(func() {
		content := []byte("hello world\n")
		h := sha256.Sum256(content)
		expectedHex = hex.EncodeToString(h[:])
		tmpFile = writeTestFile(content)
	})

	It("succeeds with valid checksum", func() {
		Expect(updater.VerifyFileChecksum(tmpFile, expectedHex)).To(Succeed())
	})

	It("succeeds with uppercase checksum", func() {
		Expect(updater.VerifyFileChecksum(tmpFile, strings.ToUpper(expectedHex))).To(Succeed())
	})

	It("fails with invalid checksum", func() {
		wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

		err := updater.VerifyFileChecksum(tmpFile, wrongHash)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("checksum mismatch"))
	})

	It("fails for nonexistent file", func() {
		err := updater.VerifyFileChecksum("/nonexistent/path", expectedHex)
		Expect(err).To(HaveOccurred())
	})
})

func writeTestFile(content []byte) string {
	f, err := os.CreateTemp(GinkgoT().TempDir(), "checksum-test-*")
	Expect(err).NotTo(HaveOccurred())

	_, err = f.Write(content)
	Expect(err).NotTo(HaveOccurred())

	Expect(f.Close()).To(Succeed())

	return f.Name()
}
