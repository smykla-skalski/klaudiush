package releaseconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

type goreleaserConfig struct {
	Changelog struct {
		Use    string `yaml:"use"`
		Format string `yaml:"format"`
	} `yaml:"changelog"`
	Release struct {
		Header string `yaml:"header"`
	} `yaml:"release"`
}

func TestGoReleaserChangelogFormatLinksCommitsWithoutEmptyPrefix(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)

	if config.Changelog.Use != "github" {
		t.Fatalf("changelog.use = %q, want github", config.Changelog.Use)
	}

	if config.Changelog.Format == "" {
		t.Fatal("changelog.format must be set explicitly")
	}

	if strings.Contains(config.Changelog.Format, "{{ .SHA }}:") {
		t.Fatalf("changelog.format still relies on the raw SHA prefix: %q", config.Changelog.Format)
	}

	tmpl, err := template.New("changelog").Option("missingkey=error").Parse(config.Changelog.Format)
	if err != nil {
		t.Fatalf("parse changelog.format: %v", err)
	}

	commit := map[string]any{
		"SHA":     "97156d6d2bd9f1d7ac76c4d6fcfe8bc8e1234567",
		"Message": "feat(hooks): normalize provider-aware hook runtime",
		"Author": map[string]any{
			"Username": "bartsmykla",
		},
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, commit); err != nil {
		t.Fatalf("render changelog.format: %v", err)
	}

	got := rendered.String()

	if strings.HasPrefix(got, ": ") {
		t.Fatalf("rendered changelog item has an empty prefix: %q", got)
	}

	wantCommitLink := "[`97156d6`](https://github.com/smykla-skalski/klaudiush/commit/97156d6d2bd9f1d7ac76c4d6fcfe8bc8e1234567):"

	if !strings.Contains(got, wantCommitLink) {
		t.Fatalf("rendered changelog item is missing the linked short SHA: %q", got)
	}

	if !strings.Contains(got, commit["Message"].(string)) {
		t.Fatalf("rendered changelog item is missing the commit message: %q", got)
	}

	if !strings.Contains(got, "(@bartsmykla)") {
		t.Fatalf("rendered changelog item is missing the GitHub login: %q", got)
	}
}

func TestGoReleaserChangelogFormatHandlesMissingAuthor(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)

	tmpl, err := template.New("changelog").Option("missingkey=error").Parse(config.Changelog.Format)
	if err != nil {
		t.Fatalf("parse changelog.format: %v", err)
	}

	commit := map[string]any{
		"SHA":     "9b5d02d88ba452c6ddaf867b071ae34fc01fdb2f",
		"Message": "feat(gemini): add init and doctor integration",
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, commit); err != nil {
		t.Fatalf("render changelog.format without author: %v", err)
	}

	got := rendered.String()

	if strings.Contains(got, "(@") {
		t.Fatalf("rendered changelog item unexpectedly contains an author suffix: %q", got)
	}

	if !strings.Contains(got, commit["Message"].(string)) {
		t.Fatalf("rendered changelog item is missing the commit message: %q", got)
	}
}

func TestGoReleaserReleaseHeaderMentionsSupportedProviders(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)

	if !strings.Contains(config.Release.Header, "Claude Code") {
		t.Fatalf("release.header must mention Claude Code: %q", config.Release.Header)
	}

	if !strings.Contains(config.Release.Header, "Codex") {
		t.Fatalf("release.header must mention Codex: %q", config.Release.Header)
	}

	if !strings.Contains(config.Release.Header, "Gemini") {
		t.Fatalf("release.header must mention Gemini: %q", config.Release.Header)
	}
}

func loadGoReleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	path := filepath.Join("..", "..", ".goreleaser.yml")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var config goreleaserConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}

	return config
}
