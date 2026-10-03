package harness

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Fixture sources.
const (
	SourceLive = "live"
	SourceDocs = "docs"
)

// Fixture is one hook invocation: the payload a harness sent, the klaudiush
// arguments its hook config ran, and the response klaudiush gave. Live
// fixtures are captured by the live suite against a real harness binary;
// docs fixtures are written from the provider's published payload reference
// for harnesses that cannot run locally. Paths are redacted to {{WORK}} and
// {{HOME}} so a fixture replays in any directory.
type Fixture struct {
	Provider       hook.Provider     `json:"provider"`
	Harness        string            `json:"harness"`
	HarnessVersion string            `json:"harness_version"`
	Source         string            `json:"source"`
	Reference      string            `json:"reference,omitempty"`
	Scenario       string            `json:"scenario"`
	Event          string            `json:"event"`
	Args           []string          `json:"args"`
	Expect         Outcome           `json:"expect"`
	Config         string            `json:"config,omitempty"`
	Workspace      map[string]string `json:"workspace,omitempty"`
	ReplaySkip     string            `json:"replay_skip,omitempty"`
	Payload        json.RawMessage   `json:"payload"`
	Response       json.RawMessage   `json:"response,omitempty"`
	path           string
}

// Path is the file the fixture was loaded from.
func (f Fixture) Path() string { return f.path }

// leakPatterns catch credentials and machine-specific paths that must never
// reach a committed fixture.
var leakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{16,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`(?i)\\?"(access|refresh|id)_token\\?"`),
	regexp.MustCompile(`/(Users|home)/[^/"{}\s]+/`),
	regexp.MustCompile(`(^|[^A-Za-z0-9._-])/(private/)?(var/folders|tmp)/`),
}

// Validate checks the fixture against the capability table: the event is
// one the provider fires today, the payload has the shape the provider
// sends, the recorded response has only fields the provider accepts and asks
// for the expected outcome, and nothing secret or machine-specific leaked.
func (f Fixture) Validate() error {
	if f.Source != SourceLive && f.Source != SourceDocs {
		return errors.Newf("%s: unknown source %q", f.path, f.Source)
	}

	if f.HarnessVersion == "" || f.Scenario == "" || len(f.Args) == 0 {
		return errors.Newf("%s: harness_version, scenario and args are required", f.path)
	}

	if err := CheckPayload(f.Provider, f.Event, f.Payload); err != nil {
		return errors.Wrapf(err, "%s", f.path)
	}

	if f.Source == SourceLive && f.Response == nil {
		return errors.Newf("%s: live fixtures record the response", f.path)
	}

	if f.Response != nil {
		if err := f.CheckResponse(f.Response); err != nil {
			return err
		}
	}

	return f.checkLeaks()
}

// CheckResponse checks a response to this fixture's payload: accepted
// fields only, and the outcome the fixture expects.
func (f Fixture) CheckResponse(response []byte) error {
	if err := CheckResponse(f.Provider, f.Event, unwrapResponse(response)); err != nil {
		return errors.Wrapf(err, "%s", f.path)
	}

	outcome, err := ClassifyResponse(unwrapResponse(response))
	if err != nil {
		return errors.Wrapf(err, "%s", f.path)
	}

	if outcome != f.Expect {
		return errors.Newf("%s: response outcome %q, want %q", f.path, outcome, f.Expect)
	}

	return nil
}

func (f Fixture) checkLeaks() error {
	data, err := json.Marshal(f)
	if err != nil {
		return errors.Wrap(err, "encoding fixture")
	}

	for _, pattern := range leakPatterns {
		if match := pattern.Find(data); match != nil {
			return errors.Newf("%s: fixture leaks %q (pattern %s)", f.path, match, pattern)
		}
	}

	return nil
}

// unwrapResponse turns a JSON null (a clean pass recorded as null) into an
// empty response.
func unwrapResponse(response []byte) []byte {
	if bytes.Equal(bytes.TrimSpace(response), []byte("null")) {
		return nil
	}

	return response
}

// Expand replaces the path placeholders in text.
func Expand(text, work, home string) string {
	return strings.NewReplacer(placeholderWork, work, placeholderHome, home).Replace(text)
}

// LoadFixtures reads every *.json fixture below dir, sorted by path.
func LoadFixtures(dir string) ([]Fixture, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.Wrap(err, "opening fixtures")
	}

	defer func() { _ = root.Close() }()

	var fixtures []Fixture

	err = fs.WalkDir(root.FS(), ".", func(rel string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(rel) != ".json" {
			return err
		}

		data, err := root.ReadFile(rel)
		if err != nil {
			return errors.Wrapf(err, "reading %s", rel)
		}

		var fixture Fixture

		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&fixture); err != nil {
			return errors.Wrapf(err, "decoding %s", rel)
		}

		fixture.path = filepath.Join(dir, rel)
		fixtures = append(fixtures, fixture)

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "loading fixtures")
	}

	slices.SortFunc(fixtures, func(a, b Fixture) int { return strings.Compare(a.path, b.path) })

	return fixtures, nil
}

// WriteFixture stores a fixture as dir/<provider>/<scenario>-<event>.json.
func WriteFixture(dir string, fixture Fixture) (string, error) {
	name := strings.ToLower(fixture.Scenario + "-" + strings.ReplaceAll(fixture.Event, ".", "-"))
	path := filepath.Join(dir, string(fixture.Provider), name+".json")

	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(fixture); err != nil {
		return "", errors.Wrap(err, "encoding fixture")
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return "", errors.Wrap(err, "creating fixture directory")
	}

	return path, errors.Wrap(os.WriteFile(path, buf.Bytes(), filePerm), "writing fixture")
}

// PromoteFixtures replaces one provider's fixtures in dir with the ones
// staged for it, so a provider is rewritten only after its whole live run
// passed and the others keep their coverage. The staged set is copied next
// to the old one first, so a failed copy leaves the old fixtures in place.
func PromoteFixtures(stage, dir string, provider hook.Provider) error {
	src := filepath.Join(stage, string(provider))
	dst := filepath.Join(dir, string(provider))
	next := dst + ".next"

	if err := os.RemoveAll(next); err != nil {
		return errors.Wrap(err, "clearing staged fixture copy")
	}

	_, statErr := os.Stat(src)

	switch {
	case statErr == nil:
		if err := os.CopyFS(next, os.DirFS(src)); err != nil {
			_ = os.RemoveAll(next)

			return errors.Wrapf(err, "copying staged %s fixtures", provider)
		}
	case !errors.Is(statErr, fs.ErrNotExist):
		return errors.Wrapf(statErr, "reading staged %s fixtures", provider)
	}

	if err := os.RemoveAll(dst); err != nil {
		return errors.Wrapf(err, "removing old %s fixtures", provider)
	}

	if _, err := os.Stat(next); errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return errors.Wrapf(os.Rename(next, dst), "replacing %s fixtures", provider)
}
