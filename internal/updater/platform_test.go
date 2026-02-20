package updater_test

import (
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/updater"
)

var _ = Describe("DetectPlatform", func() {
	It("returns current OS and architecture", func() {
		p := updater.DetectPlatform()
		Expect(p.OS).To(Equal(runtime.GOOS))
		Expect(p.Arch).To(Equal(runtime.GOARCH))
	})
})

var _ = Describe("Platform", func() {
	Describe("ArchiveName", func() {
		DescribeTable("returns correct archive name",
			func(os, arch, version, expected string) {
				p := updater.Platform{OS: os, Arch: arch}
				Expect(p.ArchiveName(version)).To(Equal(expected))
			},
			Entry("darwin arm64",
				"darwin", "arm64", "1.13.0",
				"klaudiush_1.13.0_darwin_arm64.tar.gz",
			),
			Entry("linux amd64",
				"linux", "amd64", "1.13.0",
				"klaudiush_1.13.0_linux_amd64.tar.gz",
			),
			Entry("windows amd64",
				"windows", "amd64", "1.13.0",
				"klaudiush_1.13.0_windows_amd64.zip",
			),
			Entry("windows arm64",
				"windows", "arm64", "2.0.0",
				"klaudiush_2.0.0_windows_arm64.zip",
			),
		)
	})

	Describe("IsWindows", func() {
		It("returns true for windows", func() {
			p := updater.Platform{OS: "windows", Arch: "amd64"}
			Expect(p.IsWindows()).To(BeTrue())
		})

		It("returns false for darwin", func() {
			p := updater.Platform{OS: "darwin", Arch: "arm64"}
			Expect(p.IsWindows()).To(BeFalse())
		})
	})
})

var _ = Describe("DownloadURL", func() {
	It("returns correct URL", func() {
		url := updater.DownloadURL("v1.13.0", "klaudiush_1.13.0_darwin_arm64.tar.gz")
		Expect(url).To(Equal(
			"https://github.com/smykla-skalski/klaudiush/releases/download/v1.13.0/klaudiush_1.13.0_darwin_arm64.tar.gz",
		))
	})
})

var _ = Describe("ReleaseURL", func() {
	It("returns URL containing the tag", func() {
		url := updater.ReleaseURL("v1.13.0")
		Expect(url).To(ContainSubstring("v1.13.0"))
		Expect(url).To(HavePrefix("https://github.com/"))
	})
})
