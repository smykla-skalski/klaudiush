package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// The live suite runs only with KLAUDIUSH_HARNESS_LIVE=1 (mise run
// test:harness). CI has neither the harness binaries nor a reason to run
// them, so by default every spec here is skipped.
const (
	envLive     = "KLAUDIUSH_HARNESS_LIVE"
	envBinary   = "KLAUDIUSH_HARNESS_BINARY"
	envReport   = "KLAUDIUSH_HARNESS_REPORT"
	envBase     = "KLAUDIUSH_HARNESS_TMPDIR"
	envKeep     = "KLAUDIUSH_HARNESS_KEEP"
	envOnly     = "KLAUDIUSH_HARNESS_ONLY"
	envFixtures = "KLAUDIUSH_HARNESS_UPDATE_FIXTURES"

	fixtureDir = "testdata/fixtures"
)

// The live state lives at package level so AfterSuite can write the report
// and check the real home even when the last specs were skipped, which keeps
// an Ordered container's AfterAll from running.
var (
	runner harness.Runner
	report *harness.Report
	guard  *harness.HomeGuard

	// fixtureStage collects captured fixtures during the run. A provider's
	// fixtures are replaced from it only when every scenario of that harness
	// ran and none failed, so a skipped or failing harness keeps its old ones.
	fixtureStage string
	cleanRun     = map[hook.Provider]bool{}
)

var _ = AfterSuite(func() {
	if report == nil {
		return
	}

	report.Summary(GinkgoWriter)

	path := os.Getenv(envReport)
	if path == "" {
		path = filepath.Join(os.TempDir(), "klaudiush-harness-report.json")
	}

	Expect(report.Write(path)).To(Succeed())
	GinkgoWriter.Printf("harness report: %s\n", path)

	if fixtureStage != "" {
		for provider, clean := range cleanRun {
			if clean {
				Expect(harness.PromoteFixtures(fixtureStage, fixtureDir, provider)).To(Succeed())
				GinkgoWriter.Printf("rewrote %s fixtures\n", provider)
			}
		}

		Expect(os.RemoveAll(fixtureStage)).To(Succeed())
	}

	changed, err := guard.Changed()
	Expect(err).NotTo(HaveOccurred())
	Expect(changed).To(BeEmpty(), "a live run changed real-home files")
})

var _ = Describe("Live harness enforcement", Ordered, ContinueOnFailure, Label("live"), func() {
	BeforeAll(func() {
		if os.Getenv(envLive) != "1" {
			Skip("live harness checks run only with " + envLive + "=1 (mise run test:harness)")
		}

		binary := os.Getenv(envBinary)
		Expect(binary).NotTo(BeEmpty(), envBinary+" must name the klaudiush build under test")
		Expect(filepath.IsAbs(binary)).To(BeTrue(), envBinary+" must be absolute")

		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())

		guard, err = harness.GuardHome(home, os.Getenv("XDG_CONFIG_HOME"))
		Expect(err).NotTo(HaveOccurred())

		runner = harness.Runner{
			Binary: binary,
			Base:   os.Getenv(envBase),
			Keep:   os.Getenv(envKeep) == "1",
		}
		report = harness.NewReport(klaudiushVersion(binary))

		if os.Getenv(envFixtures) == "1" {
			fixtureStage, err = os.MkdirTemp(os.Getenv(envBase), "klaudiush-harness-fixtures-")
			Expect(err).NotTo(HaveOccurred())
		}
	})

	for _, driver := range []harness.Driver{
		harness.NewClaudeDriver(),
		harness.NewCodexDriver(),
		harness.NewOpenCodeDriver(),
	} {
		Context(driver.Name(), func() {
			var version string

			BeforeAll(func() {
				entry := report.Harness(driver.Name(), driver.Provider())
				entry.Binary = driver.Binary()

				if only := os.Getenv(envOnly); only != "" &&
					!slices.Contains(strings.Split(only, ","), driver.Name()) {
					entry.Reason = "not selected by " + envOnly
					Skip(entry.Reason)
				}

				if err := driver.BinaryError(); err != nil {
					entry.Reason = err.Error()
					Fail(entry.Reason)
				}

				if driver.Binary() == "" {
					entry.Reason = driver.Name() + " is not installed"
					Skip(entry.Reason)
				}

				var err error

				version, err = probeVersion(runner.Base, driver.Binary())
				if err != nil {
					entry.Reason = err.Error()
					Skip(entry.Reason)
				}

				entry.Version = version
				entry.Status = harness.StatusRan

				if setter, ok := driver.(interface{ SetVersion(string) }); ok {
					setter.SetVersion(version)
				}

				if gap := driver.KnownGap(version); gap != "" {
					entry.Status = harness.StatusKnownGap
					entry.Reason = gap

					return
				}

				cleanRun[driver.Provider()] = true
			})

			AfterEach(func() {
				if CurrentSpecReport().Failed() {
					cleanRun[driver.Provider()] = false
				}
			})

			for _, scenario := range harness.Scenarios() {
				It(scenario.Name, func(ctx SpecContext) {
					runScenario(ctx, runner, report, driver, version, scenario)
				}, SpecTimeout(5*time.Minute))
			}
		})
	}
})

func runScenario(
	ctx context.Context,
	runner harness.Runner,
	report *harness.Report,
	driver harness.Driver,
	version string,
	scenario harness.Scenario,
) {
	started := time.Now()
	record := func(status, detail string) {
		report.Add(driver.Name(), harness.ScenarioResult{
			Scenario: scenario.Name, Status: status, Detail: detail,
			Seconds: time.Since(started).Round(time.Millisecond).Seconds(),
		})
	}

	for _, feature := range scenario.Features {
		if !driver.Supports(feature) {
			record(harness.StatusSkipped, "unsupported: "+string(feature))
			Skip(driver.Name() + " does not support " + string(feature))
		}
	}

	gap := driver.KnownGap(version)
	if gap != "" && scenario.Name != "deny_shell" {
		record(harness.StatusKnownGap, gap)
		Skip(gap)
	}

	result, cleanup, err := runner.Run(ctx, driver, version, scenario)
	DeferCleanup(cleanup)

	if err != nil {
		record(harness.StatusFailed, err.Error())
		Fail(err.Error())
	}

	if gap != "" {
		if problems := result.HarnessProblems(); len(problems) > 0 {
			detail := strings.Join(problems, "; ")
			record(harness.StatusFailed, detail)
			Fail(detail + "\nharness output:\n" + string(result.Output))
		}

		if !result.GapConfirmed() {
			detail := "known gap did not reproduce as recorded: " +
				strings.Join(result.Problems(), "; ")
			record(harness.StatusFailed, detail)
			Fail(driver.Name() + " " + version + ": " + detail)
		}

		record(harness.StatusKnownGap, gap)
		Skip("confirmed known gap: " + gap)
	}

	if problems := result.Problems(); len(problems) > 0 {
		detail := strings.Join(problems, "; ")
		record(harness.StatusFailed, detail)
		Fail(detail + "\nharness output:\n" + string(result.Output))
	}

	if fixtureStage != "" {
		fixtures, err := result.Fixtures()
		Expect(err).NotTo(HaveOccurred())

		for _, fixture := range fixtures {
			Expect(fixture.Validate()).To(Succeed())

			_, err := harness.WriteFixture(fixtureStage, fixture)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	record(harness.StatusPassed, "")
}

func probeVersion(base, binary string) (string, error) {
	sb, err := harness.NewSandbox(base)
	if err != nil {
		return "", err
	}

	defer func() { _ = sb.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	return harness.Version(ctx, sb, binary)
}

func klaudiushVersion(binary string) string {
	sb, err := harness.NewSandbox(os.Getenv(envBase))
	if err != nil {
		return "unknown"
	}

	defer func() { _ = sb.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := harness.RunIn(ctx, sb, sb.Work, binary, "version")
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}
