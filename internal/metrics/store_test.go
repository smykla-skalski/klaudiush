package metrics_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/filelock"
	"github.com/smykla-skalski/klaudiush/internal/metrics"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const secretCommand = "git commit -sS -m 'feat(api): token ghp_abcdefghijklmnopqrstuvwxyz0123456789'"

var _ = Describe("Store", func() {
	var (
		dir   string
		path  string
		now   time.Time
		store *metrics.Store
	)

	newStore := func(cfg *config.MetricsConfig) *metrics.Store {
		return metrics.NewStore(cfg,
			metrics.WithPath(path),
			metrics.WithTimeFunc(func() time.Time { return now }),
			metrics.WithLockTimeout(5*time.Second),
		)
	}

	preTool := func(session, command string) *hook.Context {
		return &hook.Context{
			Provider:     hook.ProviderClaude,
			Event:        hook.CanonicalEventBeforeTool,
			RawEventName: "PreToolUse",
			EventType:    hook.EventTypePreToolUse,
			ToolName:     hook.ToolTypeBash,
			SessionID:    session,
			ToolInput:    hook.ToolInput{Command: command},
		}
	}

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		path = filepath.Join(dir, "metrics", "outcomes.jsonl")
		now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
		store = newStore(nil)
	})

	It("keeps no command, message, path or session ID", func() {
		file := filepath.Join(dir, "very-private-project", "notes.md")

		Expect(store.Record(&metrics.Observation{
			Context: &hook.Context{
				Provider:     hook.ProviderClaude,
				Event:        hook.CanonicalEventAfterTool,
				RawEventName: "PostToolUse",
				ToolName:     hook.ToolTypeWrite,
				ToolFamily:   hook.ToolFamilyWrite,
				SessionID:    "session-1234-secret",
				ToolInput:    hook.ToolInput{FilePath: file, Command: secretCommand},
			},
			Errors: []*dispatcher.ValidationError{{
				Validator: "validate-markdown",
				Message:   "ghp_abcdefghijklmnopqrstuvwxyz0123456789 found",
				Reference: validator.RefGitMissingFlags,
				Resource:  hook.ResourceFilePrefix + file,
			}},
			Checks: []dispatcher.Check{{Validator: "validate-markdown", Resource: "file:" + file}},
		})).To(Succeed())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		text := string(data)
		Expect(text).NotTo(ContainSubstring("ghp_"))
		Expect(text).NotTo(ContainSubstring("very-private-project"))
		Expect(text).NotTo(ContainSubstring("notes.md"))
		Expect(text).NotTo(ContainSubstring("session-1234-secret"))
		Expect(text).NotTo(ContainSubstring("git commit"))
		Expect(text).To(ContainSubstring(`"c":"GIT010"`))

		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	})

	It("reduces crafted names to short safe tokens", func() {
		Expect(store.Record(&metrics.Observation{
			Context: &hook.Context{
				Provider:     hook.ProviderClaude,
				Event:        hook.CanonicalEventBeforeTool,
				RawEventName: "Pre\"ToolUse\n" + strings.Repeat("x", 200),
			},
			Errors: []*dispatcher.ValidationError{{
				Validator:   "plugin \"evil\"\n{}",
				ShouldBlock: true,
			}},
			Stopped: true,
		})).To(Succeed())

		records, skipped, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(skipped).To(BeZero())
		Expect(records).To(HaveLen(1))
		Expect(records[0].Event).To(HaveLen(64))
		Expect(records[0].Event).To(HavePrefix("Pre_ToolUse_"))
		Expect(records[0].Findings[0].Validator).To(Equal("plugin__evil____"))
	})

	It("keys the same session and resource alike, and other sessions apart", func() {
		for _, session := range []string{"a", "a", "b"} {
			obs := &metrics.Observation{Context: preTool(session, "ls")}
			Expect(store.Record(obs)).To(Succeed())
		}

		records, _, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(3))
		Expect(records[0].Session).To(Equal(records[1].Session))
		Expect(records[0].Session).NotTo(Equal(records[2].Session))
		Expect(records[0].Resource).To(Equal(records[2].Resource))
	})

	It("rotates past the size cap and keeps a single backup", func() {
		store = newStore(&config.MetricsConfig{MaxFileSizeMB: 1})
		big := make([]*dispatcher.ValidationError, 0, 40)

		for range 40 {
			big = append(big, &dispatcher.ValidationError{
				Validator:   strings.Repeat("v", 60),
				Reference:   validator.RefGitMissingFlags,
				ShouldBlock: true,
				Resource:    "command",
			})
		}

		for range 900 {
			Expect(store.Record(&metrics.Observation{
				Context: preTool("s", "ls"),
				Errors:  big,
				Stopped: true,
			})).To(Succeed())
		}

		entries, err := os.ReadDir(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())

		var total int64

		for _, entry := range entries {
			Expect(entry.Name()).To(BeElementOf("outcomes.jsonl", "outcomes.jsonl.1",
				"outcomes.jsonl.lock", "salt"))

			info, infoErr := entry.Info()
			Expect(infoErr).NotTo(HaveOccurred())

			total += info.Size()
		}

		Expect(total).To(BeNumerically("<=", 2<<20+4096))

		records, skipped, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(skipped).To(BeZero())
		Expect(len(records)).To(BeNumerically("<", 900))
		Expect(records[0].Findings).To(HaveLen(32))
	})

	It("loses no line under concurrent writers", func() {
		const writers, each = 16, 25

		var wg sync.WaitGroup

		for w := range writers {
			wg.Go(func() {
				defer GinkgoRecover()

				writer := newStore(nil)
				for range each {
					Expect(writer.Record(&metrics.Observation{
						Context: preTool(string(rune('a'+w)), "ls"),
					})).To(Succeed())
				}
			})
		}

		wg.Wait()

		records, skipped, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(skipped).To(BeZero())
		Expect(records).To(HaveLen(writers * each))
	})

	It("skips unreadable lines and records outside the window", func() {
		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		now = now.Add(48 * time.Hour)

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.WriteString("{\"t\":\n\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		records, skipped, err := store.Load(now.Add(-time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(1))
		Expect(skipped).To(Equal(1))

		file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.WriteString(`{"t":"2026-10-05T12:`)
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		records, skipped, err = store.Load(now.Add(-time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(2))
		Expect(skipped).To(Equal(2))
	})

	It("prunes records older than the retention", func() {
		store = newStore(&config.MetricsConfig{Retention: config.Duration(24 * time.Hour)})

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		now = now.Add(48 * time.Hour)

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		dropped, err := store.Prune()
		Expect(err).NotTo(HaveOccurred())
		Expect(dropped).To(Equal(1))

		dropped, err = store.Prune()
		Expect(err).NotTo(HaveOccurred())
		Expect(dropped).To(BeZero())

		records, _, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(1))
		Expect(store.Retention()).To(Equal(24 * time.Hour))
	})

	It("clears the logs and the salt, so new keys cannot be linked", func() {
		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		before, _, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())

		Expect(store.Clear()).To(Succeed())
		Expect(filepath.Join(filepath.Dir(path), "salt")).NotTo(BeAnExistingFile())
		Expect(path).NotTo(BeAnExistingFile())
		Expect(store.Clear()).To(Succeed())

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())

		after, _, err := store.Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(HaveLen(1))
		Expect(after[0].Session).NotTo(Equal(before[0].Session))
	})

	It("gives up when another writer holds the lock", func() {
		store = metrics.NewStore(
			nil,
			metrics.WithPath(path),
			metrics.WithLockTimeout(time.Millisecond),
		)

		Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())

		lock, err := filelock.Acquire(path+".lock", time.Second)
		Expect(err).NotTo(HaveOccurred())

		err = store.Record(&metrics.Observation{Context: preTool("s", "ls")})
		Expect(err).To(MatchError(filelock.ErrTimeout))
		Expect(lock.Release()).To(Succeed())

		Expect(store.Record(&metrics.Observation{Context: preTool("s", "ls")})).To(Succeed())
		Expect(store.Record(nil)).To(Succeed())
		Expect(store.Path()).To(Equal(path))
	})
})

var _ = Describe("Store without a writable directory", func() {
	It("still reads the logs", func() {
		if os.Geteuid() == 0 {
			Skip("root ignores directory permissions")
		}

		dir := filepath.Join(GinkgoT().TempDir(), "metrics")
		path := filepath.Join(dir, "outcomes.jsonl")

		writer := metrics.NewStore(nil, metrics.WithPath(path))
		Expect(writer.Record(&metrics.Observation{
			Context: &hook.Context{Provider: hook.ProviderClaude},
		})).To(Succeed())
		Expect(os.Remove(path + ".lock")).To(Succeed())
		Expect(os.Chmod(dir, 0o500)).To(Succeed())

		DeferCleanup(func() { Expect(os.Chmod(dir, 0o700)).To(Succeed()) })

		records, _, err := metrics.NewStore(nil, metrics.WithPath(path)).Load(time.Time{})
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(1))

		Expect(writer.Probe()).NotTo(Succeed())
	})
})
