package settings_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

type stubOpenCode struct {
	version string
	err     error
}

func (s stubOpenCode) Detect(context.Context) (string, error) {
	return s.version, s.err
}

type countingOpenCode struct {
	calls *int
}

func (c countingOpenCode) Detect(context.Context) (string, error) {
	*c.calls++

	return "2.0.19", nil
}

var _ = Describe("opencode plugin API selection", func() {
	const binaryPath = "/opt/homebrew/bin/klaudiush"

	var pluginPath string

	BeforeEach(func() {
		pluginPath = filepath.Join(GinkgoT().TempDir(), "plugin", "klaudiush.ts")
	})

	render := func(api settings.OpenCodeAPI) string {
		rendered, err := settings.RenderOpenCodePlugin(binaryPath, api)
		Expect(err).NotTo(HaveOccurred())

		return string(rendered)
	}

	Describe("the 2.x bridge", func() {
		It("embeds the binary and leaves no template directives", func() {
			source := render(settings.OpenCodeAPIV2)

			Expect(source).To(ContainSubstring(binaryPath))
			Expect(source).NotTo(ContainSubstring("{{"))
		})

		It("exports only the {id, setup} definition opencode 2.x validates", func() {
			source := render(settings.OpenCodeAPIV2)

			var exports []string

			for line := range strings.SplitSeq(source, "\n") {
				if strings.HasPrefix(line, "export") {
					exports = append(exports, line)
				}
			}

			Expect(exports).To(Equal([]string{`export default { id: "klaudiush", setup }`}))
		})

		It("refuses a tool by throwing from execute.before", func() {
			source := render(settings.OpenCodeAPIV2)

			Expect(source).To(ContainSubstring(`ctx.tool.hook("execute.before",`))
			Expect(source).To(ContainSubstring("throw new Error(blockReason(resp))"))
			Expect(source).To(ContainSubstring(`event !== "tool.execute.before"`))
			Expect(source).To(ContainSubstring("sessionMoveEscape(event.tool, event.input, cwd)"))
		})

		It("subscribes to every advertised opencode event", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV2,
			)
			Expect(err).NotTo(HaveOccurred())

			parser := settings.NewOpenCodePluginParser(pluginPath)

			for _, eventName := range hook.OpenCodeEventNames() {
				Expect(parser.HasEventHook(eventName, binaryPath)).
					To(BeTrue(), "event %s is not subscribed to", eventName)
			}

			hasHook, err := parser.HasEventHook("permission.ask", binaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasHook).To(BeFalse())
		})

		It("renders the 1.x bridge for an unknown API", func() {
			Expect(render(settings.OpenCodeAPIUnknown)).To(Equal(render(settings.OpenCodeAPIV1)))
		})
	})

	DescribeTable("OpenCodeAPIForVersion",
		func(version string, want settings.OpenCodeAPI) {
			Expect(settings.OpenCodeAPIForVersion(version)).To(Equal(want))
		},
		Entry("2.0.19", "2.0.19", settings.OpenCodeAPIV2),
		Entry("a later major", "3.1.0", settings.OpenCodeAPIV2),
		Entry("v-prefixed", "v2.0.0", settings.OpenCodeAPIV2),
		Entry("1.x", "1.14.0", settings.OpenCodeAPIV1),
		Entry("0.x", "0.15.3", settings.OpenCodeAPIV1),
		Entry("empty", "", settings.OpenCodeAPIUnknown),
		Entry("garbage", "nightly", settings.OpenCodeAPIUnknown),
	)

	DescribeTable("OpenCodeAPIVerified",
		func(version string, want bool) {
			Expect(settings.OpenCodeAPIVerified(version)).To(Equal(want))
		},
		Entry("2.x", "2.0.19", true),
		Entry("1.x", "1.14.0", true),
		Entry("a later major", "3.0.0", false),
		Entry("garbage", "nightly", false),
	)

	It("runs a cached detector once", func() {
		calls := 0
		cached := settings.NewCachedOpenCodeVersionDetector(countingOpenCode{calls: &calls})

		for range 3 {
			Expect(cached.Detect(context.Background())).To(Equal("2.0.19"))
		}

		Expect(calls).To(Equal(1))
	})

	DescribeTable(
		"DetectOpenCodePluginAPI",
		func(source string, want settings.OpenCodeAPI) {
			Expect(settings.DetectOpenCodePluginAPI(source)).To(Equal(want))
		},
		Entry(
			"named plugin function",
			"export const Klaudiush = async () => ({})",
			settings.OpenCodeAPIV1,
		),
		Entry("exported function", "export async function Plugin() {}", settings.OpenCodeAPIV1),
		Entry("default definition", "export default { id: \"k\", setup }", settings.OpenCodeAPIV2),
		Entry("default without setup", "export default { id: \"k\" }", settings.OpenCodeAPIUnknown),
		Entry(
			"both export styles",
			"export const A = 1\nexport default { setup }",
			settings.OpenCodeAPIUnknown,
		),
		Entry("no exports", "const a = 1", settings.OpenCodeAPIUnknown),
	)

	It("classifies both shipped templates", func() {
		Expect(settings.DetectOpenCodePluginAPI(render(settings.OpenCodeAPIV1))).
			To(Equal(settings.OpenCodeAPIV1))
		Expect(settings.DetectOpenCodePluginAPI(render(settings.OpenCodeAPIV2))).
			To(Equal(settings.OpenCodeAPIV2))
	})

	Describe("CommandOpenCodeVersionDetector", func() {
		var (
			ctrl   *gomock.Controller
			tools  *execpkg.MockToolChecker
			runner *execpkg.MockCommandRunner
			ctx    context.Context
		)

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			tools = execpkg.NewMockToolChecker(ctrl)
			runner = execpkg.NewMockCommandRunner(ctrl)
			ctx = context.Background()
		})

		detect := func() (string, error) {
			return settings.NewOpenCodeVersionDetectorWith(tools, runner).Detect(ctx)
		}

		It("reports a missing opencode", func() {
			tools.EXPECT().IsAvailable("opencode").Return(false)

			_, err := detect()
			Expect(err).To(MatchError(settings.ErrOpenCodeNotInstalled))
		})

		DescribeTable("extracts the version",
			func(stdout, want string) {
				tools.EXPECT().IsAvailable("opencode").Return(true)
				runner.EXPECT().Run(ctx, "opencode", "--version").
					Return(execpkg.CommandResult{Stdout: stdout})

				Expect(detect()).To(Equal(want))
			},
			Entry("bare", "2.0.19\n", "2.0.19"),
			Entry("with a name", "opencode 1.14.0\n", "1.14.0"),
			Entry("prerelease", "2.1.0-beta.3\n", "2.1.0-beta.3"),
		)

		It("reports a failing opencode", func() {
			tools.EXPECT().IsAvailable("opencode").Return(true)
			runner.EXPECT().Run(ctx, "opencode", "--version").
				Return(execpkg.CommandResult{Err: errors.New("exit 1"), Stderr: "broken"})

			_, err := detect()
			Expect(err).To(MatchError(ContainSubstring("broken")))
		})

		It("reports output without a version", func() {
			tools.EXPECT().IsAvailable("opencode").Return(true)
			runner.EXPECT().Run(ctx, "opencode", "--version").
				Return(execpkg.CommandResult{Stdout: "hello"})

			_, err := detect()
			Expect(err).To(MatchError(ContainSubstring("no version")))
		})

		It("takes the last version printed, after any banner", func() {
			tools.EXPECT().IsAvailable("opencode").Return(true)
			runner.EXPECT().Run(ctx, "opencode", "--version").
				Return(execpkg.CommandResult{Stdout: "update 2.1.0 available\n2.0.19\n"})

			Expect(detect()).To(Equal("2.0.19"))
		})

		It("falls back to an executable outside PATH", func() {
			dir := GinkgoT().TempDir()
			missing := filepath.Join(dir, "missing", "opencode")
			notExecutable := filepath.Join(dir, "plain")
			executable := filepath.Join(dir, "opencode")

			Expect(os.WriteFile(notExecutable, []byte(""), 0o600)).To(Succeed())
			Expect(os.WriteFile(executable, []byte(""), 0o700)).To(Succeed())

			tools.EXPECT().IsAvailable("opencode").Return(false)
			runner.EXPECT().Run(ctx, executable, "--version").
				Return(execpkg.CommandResult{Stdout: "2.0.19\n"})

			version, err := settings.NewOpenCodeVersionDetectorWith(
				tools, runner, missing, dir, notExecutable, executable,
			).Detect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal("2.0.19"))
		})

		It("is built from the real PATH by default", func() {
			Expect(settings.NewOpenCodeVersionDetector()).NotTo(BeNil())
		})
	})

	Describe("ResolveOpenCodeTarget", func() {
		resolve := func(detector settings.OpenCodeVersionDetector) settings.OpenCodeTarget {
			return settings.ResolveOpenCodeTarget(context.Background(), detector, pluginPath)
		}

		It("follows the installed opencode", func() {
			target := resolve(stubOpenCode{version: "2.0.19"})

			Expect(target.API).To(Equal(settings.OpenCodeAPIV2))
			Expect(target.Detected()).To(BeTrue())
			Expect(target.Describe()).To(Equal("opencode 2.0.19"))
		})

		It("falls back to 1.x on a fresh install without opencode", func() {
			target := resolve(stubOpenCode{err: settings.ErrOpenCodeNotInstalled})

			Expect(target.API).To(Equal(settings.OpenCodeAPIV1))
			Expect(target.Detected()).To(BeFalse())
			Expect(target.DetectErr).To(MatchError(settings.ErrOpenCodeNotInstalled))
			Expect(target.Describe()).To(Equal("opencode 1.x (version not detected)"))
		})

		It("keeps the API of the installed plugin when opencode is not detected", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV2,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(resolve(stubOpenCode{err: settings.ErrOpenCodeNotInstalled}).API).
				To(Equal(settings.OpenCodeAPIV2))
		})

		It("treats an unrecognized version as undetected", func() {
			Expect(os.MkdirAll(filepath.Dir(pluginPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(pluginPath, []byte("const x = 1\n"), 0o600)).To(Succeed())

			target := resolve(stubOpenCode{version: "nightly"})

			Expect(target.API).To(Equal(settings.OpenCodeAPIV1))
			Expect(target.DetectErr).To(MatchError(ContainSubstring("nightly")))
		})
	})
})
