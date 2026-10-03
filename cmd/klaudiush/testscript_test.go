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

	// Set HOME to the work directory so logger can create the log file
	env.Setenv("HOME", env.WorkDir)

	// The directory testscript installs klaudiush into, so a script can run
	// it with a PATH that holds nothing else
	testBin, _, _ := strings.Cut(env.Getenv("PATH"), string(os.PathListSeparator))
	env.Setenv("KLAUDIUSH_TEST_BIN", testBin)

	// Force CLI git implementation in tests to avoid singleton caching issues
	// The SDK implementation uses a singleton cache that persists across test runs
	env.Setenv("KLAUDIUSH_USE_SDK_GIT", "false")

	return nil
}

func TestScriptDispatcher(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/dispatcher",
		Setup: setupTestEnv,
	})
}

func TestScriptInit(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/init",
		Setup: setupTestEnv,
	})
}

func TestScriptDoctor(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/doctor",
		Setup: setupTestEnv,
	})
}

func TestScriptMarkdown(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/markdown",
		Setup: setupTestEnv,
	})
}

func TestScriptDebug(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/debug",
		Setup: setupTestEnv,
	})
}

func TestScriptBypass(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/scripts/bypass",
		Setup: setupTestEnv,
	})
}
