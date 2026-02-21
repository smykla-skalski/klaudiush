package updater_test

import (
	"context"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/updater"
)

var _ = Describe("BrewUpdater", func() {
	brewInfoJSON := func(installed, stable string) string {
		return `{"formulae":[{"versions":{"stable":"` + stable +
			`"},"installed":[{"version":"` + installed + `"}]}]}`
	}

	Describe("CheckOutdated", func() {
		It("detects outdated formula", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Stdout: brewInfoJSON("1.21.0", "1.22.1"),
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			current, latest, outdated, err := bu.CheckOutdated(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(current).To(Equal("1.21.0"))
			Expect(latest).To(Equal("1.22.1"))
			Expect(outdated).To(BeTrue())
		})

		It("detects up-to-date formula", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Stdout: brewInfoJSON("1.22.1", "1.22.1"),
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			current, latest, outdated, err := bu.CheckOutdated(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(current).To(Equal("1.22.1"))
			Expect(latest).To(Equal("1.22.1"))
			Expect(outdated).To(BeFalse())
		})

		It("returns error on brew failure", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Err:      errors.New("brew not found"),
						ExitCode: 127,
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			_, _, _, err := bu.CheckOutdated(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Upgrade", func() {
		It("upgrades successfully", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Stdout: brewInfoJSON("1.21.0", "1.22.1"),
					},
					"brew upgrade smykla-skalski/tap/klaudiush": {
						Stdout: "==> Upgrading smykla-skalski/tap/klaudiush\n",
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			result, err := bu.Upgrade(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PreviousVersion).To(Equal("1.21.0"))
			Expect(result.NewVersion).To(Equal("1.22.1"))
		})

		It("returns ErrAlreadyLatest when already installed", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Stdout: brewInfoJSON("1.22.1", "1.22.1"),
					},
					"brew upgrade smykla-skalski/tap/klaudiush": {
						Stderr: "Warning: smykla-skalski/tap/klaudiush already installed\n",
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			_, err := bu.Upgrade(context.Background())
			Expect(err).To(MatchError(updater.ErrAlreadyLatest))
		})

		It("returns error on brew upgrade failure", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"brew info --json=v2 smykla-skalski/tap/klaudiush": {
						Stdout: brewInfoJSON("1.21.0", "1.22.1"),
					},
					"brew upgrade smykla-skalski/tap/klaudiush": {
						Err:      errors.New("brew error"),
						ExitCode: 1,
						Stderr:   "Error: some brew problem\n",
					},
				},
			}
			bu := updater.NewBrewUpdater(runner)

			_, err := bu.Upgrade(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("brew upgrade failed"))
		})
	})

	Describe("UpgradeToVersion", func() {
		It("always returns ErrBrewVersionPin", func() {
			runner := &stubRunner{results: map[string]exec.CommandResult{}}
			bu := updater.NewBrewUpdater(runner)

			err := bu.UpgradeToVersion(context.Background(), "v1.20.0")
			Expect(err).To(MatchError(updater.ErrBrewVersionPin))
		})
	})
})
