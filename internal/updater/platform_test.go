package updater

import (
	"runtime"
	"strings"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	p := DetectPlatform()

	if p.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", p.OS, runtime.GOOS)
	}

	if p.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", p.Arch, runtime.GOARCH)
	}
}

func TestPlatformArchiveName(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		version  string
		want     string
	}{
		{
			name:     "darwin arm64",
			platform: Platform{OS: "darwin", Arch: "arm64"},
			version:  "1.13.0",
			want:     "klaudiush_1.13.0_darwin_arm64.tar.gz",
		},
		{
			name:     "linux amd64",
			platform: Platform{OS: "linux", Arch: "amd64"},
			version:  "1.13.0",
			want:     "klaudiush_1.13.0_linux_amd64.tar.gz",
		},
		{
			name:     "windows amd64",
			platform: Platform{OS: "windows", Arch: "amd64"},
			version:  "1.13.0",
			want:     "klaudiush_1.13.0_windows_amd64.zip",
		},
		{
			name:     "windows arm64",
			platform: Platform{OS: "windows", Arch: "arm64"},
			version:  "2.0.0",
			want:     "klaudiush_2.0.0_windows_arm64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.platform.ArchiveName(tt.version)
			if got != tt.want {
				t.Errorf("ArchiveName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlatformIsWindows(t *testing.T) {
	if !(Platform{OS: "windows", Arch: "amd64"}).IsWindows() {
		t.Error("expected IsWindows() = true for windows platform")
	}

	if (Platform{OS: "darwin", Arch: "arm64"}).IsWindows() {
		t.Error("expected IsWindows() = false for darwin platform")
	}
}

func TestDownloadURL(t *testing.T) {
	url := DownloadURL("v1.13.0", "klaudiush_1.13.0_darwin_arm64.tar.gz")
	want := "https://github.com/smykla-skalski/klaudiush/releases/download/v1.13.0/klaudiush_1.13.0_darwin_arm64.tar.gz"

	if url != want {
		t.Errorf("DownloadURL() = %q, want %q", url, want)
	}
}

func TestReleaseURL(t *testing.T) {
	url := ReleaseURL("v1.13.0")

	if !strings.Contains(url, "v1.13.0") {
		t.Errorf("ReleaseURL() = %q, expected to contain v1.13.0", url)
	}

	if !strings.HasPrefix(url, "https://github.com/") {
		t.Errorf("ReleaseURL() = %q, expected to start with https://github.com/", url)
	}
}
