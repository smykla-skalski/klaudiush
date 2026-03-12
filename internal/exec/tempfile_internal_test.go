package exec

import (
	"os"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	errCreateTempFile = errors.New("create temp file")
	errWriteFailure   = errors.New("write failure")
	errCloseFailure   = errors.New("close failure")
)

var _ = Describe("TempFileManager", func() {
	var manager TempFileManager

	var (
		originalCreateTempFile = createTempFile
		originalWriteString    = writeString
		originalCloseFile      = closeFile
	)

	resetHelpers := func() {
		createTempFile = originalCreateTempFile
		writeString = originalWriteString
		closeFile = originalCloseFile
	}

	BeforeEach(func() {
		resetHelpers()

		manager = NewTempFileManager()
	})

	AfterEach(func() {
		resetHelpers()
	})

	Describe("Create", func() {
		It("creates a file with the requested content", func() {
			path, cleanup, err := manager.Create("test-*.txt", "payload")

			Expect(err).ToNot(HaveOccurred())
			Expect(path).ToNot(BeEmpty())

			defer cleanup()

			content, err := os.ReadFile(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("payload"))
		})

		It("returns error when create temp file fails", func() {
			createTempFile = func(string, string) (*os.File, error) {
				return nil, errCreateTempFile
			}

			path, cleanup, err := manager.Create("test-*.txt", "payload")
			Expect(err).To(HaveOccurred())
			Expect(path).To(BeEmpty())
			Expect(cleanup).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("creating temp file"))
		})

		It("removes file when writing content fails", func() {
			var created string

			dir := GinkgoT().TempDir()

			createTempFile = func(_ string, pattern string) (*os.File, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}

				created = file.Name()

				return file, nil
			}

			writeString = func(*os.File, string) (int, error) {
				return 0, errWriteFailure
			}

			_, _, err := manager.Create("content-*.txt", "payload")
			Expect(err).To(HaveOccurred())

			_, statErr := os.Stat(created)

			Expect(statErr).ToNot(BeNil())
			Expect(os.IsNotExist(statErr)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("writing to temp file"))
		})

		It("removes file when close fails", func() {
			var created string

			dir := GinkgoT().TempDir()

			createTempFile = func(_ string, pattern string) (*os.File, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}

				created = file.Name()

				return file, nil
			}

			closeFile = func(*os.File) error {
				return errCloseFailure
			}

			_, _, err := manager.Create("content-*.txt", "payload")
			Expect(err).To(HaveOccurred())

			_, statErr := os.Stat(created)

			Expect(statErr).ToNot(BeNil())
			Expect(os.IsNotExist(statErr)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("closing temp file"))
		})
	})
})
