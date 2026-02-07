package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
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
	debugMode = true
	traceMode = false
	configPath = ""
	globalConfig = ""
	disableList = []string{}
	globalFlag = false
	forceFlag = false
	noTUIFlag = false
	verboseFlag = false
	fixFlag = false
	categoryFlag = []string{}
	validatorFilter = ""

	// Reset git repository cache so each test discovers its own repo
	gitpkg.ResetRepositoryCache()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
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
