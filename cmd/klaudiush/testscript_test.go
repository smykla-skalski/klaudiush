package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"klaudiush": mainFunc,
	})
}

// mainFunc wraps the CLI for testscript execution.
func mainFunc() {
	// Reset flags for each invocation (Cobra reuses the same command)
	hookType = ""
	failureMode = ""
	debugMode = true
	traceMode = false
	configPath = ""
	globalConfig = ""
	disableList = []string{}
	globalFlag = false
	forceFlag = false
	noTUIFlag = false
	installHooksFlag = true
	providersFlag = nil
	codexHooksFlag = ""
	verboseFlag = false
	fixFlag = false
	categoryFlag = []string{}
	validatorFilter = ""
	bypassReason = ""
	bypassDuration = ""
	bypassGlobal = false
	bypassAll = false
	metricsSince = ""
	metricsProvider = ""
	metricsEvent = ""
	metricsJSON = false

	// Reset git repository cache so each test discovers its own repo
	gitpkg.ResetRepositoryCache()

	if code := mainWithExitCode(); code != ExitCodeAllow {
		os.Exit(code)
	}
}

// setupTestEnv creates the necessary directories and files for testscript.
func setupTestEnv(env *testscript.Env) error {
	// Create .claude/hooks directory in the work directory
	claudeHooksDir := filepath.Join(env.WorkDir, ".claude", "hooks")
	if err := os.MkdirAll(claudeHooksDir, 0o755); err != nil {
		return err
	}

	homeDir := env.WorkDir
	xdgDirs := map[string]string{
		"XDG_CACHE_HOME":  filepath.Join(homeDir, ".cache"),
		"XDG_CONFIG_HOME": filepath.Join(homeDir, ".config"),
		"XDG_DATA_HOME":   filepath.Join(homeDir, ".local", "share"),
		"XDG_RUNTIME_DIR": filepath.Join(homeDir, ".runtime"),
		"XDG_STATE_HOME":  filepath.Join(homeDir, ".local", "state"),
	}

	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return err
	}

	env.Setenv("HOME", homeDir)

	for name, dir := range xdgDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		env.Setenv(name, dir)
	}

	// The directory testscript installs klaudiush into, so a script can run
	// it with a PATH that holds nothing else
	testBin, _, _ := strings.Cut(env.Getenv("PATH"), string(os.PathListSeparator))
	env.Setenv("KLAUDIUSH_TEST_BIN", testBin)

	// Force CLI git implementation in tests to avoid singleton caching issues
	// The SDK implementation uses a singleton cache that persists across test runs
	env.Setenv("KLAUDIUSH_USE_SDK_GIT", "false")

	return nil
}

func scriptParams(t *testing.T, dir string) testscript.Params {
	t.Helper()

	return testscript.Params{
		Dir:         dir,
		Setup:       setupTestEnv,
		WorkdirRoot: t.TempDir(),
	}
}

func TestScriptDispatcher(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/dispatcher"))
}

func TestScriptInit(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/init"))
}

func TestScriptDoctor(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/doctor"))
}

func TestScriptMarkdown(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/markdown"))
}

func TestScriptDebug(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/debug"))
}

func TestScriptMetrics(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/metrics"))
}

func TestScriptBypass(t *testing.T) {
	testscript.Run(t, scriptParams(t, "testdata/scripts/bypass"))
}
