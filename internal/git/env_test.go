package git

import (
	"os"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("clearGitEnvVars", func() {
	Describe("GIT_INDEX_FILE handling", func() {
		AfterEach(func() {
			// Clean up env var after each test
			_ = os.Unsetenv("GIT_INDEX_FILE")
		})

		It("should clear GIT_INDEX_FILE when set", func() {
			// Set the environment variable
			err := os.Setenv("GIT_INDEX_FILE", "test-value")
			Expect(err).NotTo(HaveOccurred())

			// Verify it was set
			val, exists := os.LookupEnv("GIT_INDEX_FILE")
			Expect(exists).To(BeTrue())
			Expect(val).To(Equal("test-value"))

			// Call the function
			clearGitEnvVars()

			// Verify it was cleared
			_, exists = os.LookupEnv("GIT_INDEX_FILE")
			Expect(exists).To(BeFalse())
		})

		It("should not error when GIT_INDEX_FILE is not set", func() {
			// Ensure it's not set
			_ = os.Unsetenv("GIT_INDEX_FILE")

			// Should not panic
			Expect(func() { clearGitEnvVars() }).NotTo(Panic())
		})
	})

	Describe("gitEnvVarsToUnset list", func() {
		It("should contain GIT_INDEX_FILE", func() {
			Expect(slices.Contains(gitEnvVarsToUnset, "GIT_INDEX_FILE")).To(BeTrue())
		})
	})
})
