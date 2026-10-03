package metrics

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/filelock"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	fileMode = 0o600
	dirMode  = 0o700

	saltBytes = 32
	keyChars  = 16

	backupSuffix = ".1"
	keptFiles    = 2
	lockSuffix   = ".lock"
	saltFile     = "salt"
	tempPattern  = ".*.tmp"

	maxLineBytes = 1 << 20
)

// defaultLockTimeout is short: a hook waiting on metrics delays the agent,
// and a dropped sample costs less than that.
const defaultLockTimeout = 250 * time.Millisecond

// readLockTimeout is how long a report waits to open the logs.
const readLockTimeout = 5 * time.Second

// Store appends records to a size-capped JSONL log and reads them back.
// Every write holds the log's file lock, so concurrent hooks never
// interleave lines or lose a rotation.
type Store struct {
	path        string
	maxSize     int64
	retention   time.Duration
	lockTimeout time.Duration
	now         func() time.Time
}

// Option configures a Store.
type Option func(*Store)

// WithPath sets the log file.
func WithPath(path string) Option {
	return func(s *Store) {
		s.path = path
	}
}

// WithTimeFunc sets the clock.
func WithTimeFunc(now func() time.Time) Option {
	return func(s *Store) {
		s.now = now
	}
}

// WithLockTimeout sets how long a write waits for the lock.
func WithLockTimeout(timeout time.Duration) Option {
	return func(s *Store) {
		s.lockTimeout = timeout
	}
}

// NewStore returns the store for cfg, at xdg.MetricsFile unless WithPath
// says otherwise.
func NewStore(cfg *config.MetricsConfig, opts ...Option) *Store {
	s := &Store{
		path:        xdg.MetricsFile(),
		maxSize:     cfg.GetMaxFileSize(),
		retention:   cfg.GetRetention(),
		lockTimeout: defaultLockTimeout,
		now:         time.Now,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Path returns the active log file.
func (s *Store) Path() string {
	return s.path
}

// Retention returns the configured report window and prune age.
func (s *Store) Retention() time.Duration {
	return s.retention
}

// Record appends one observation. It never blocks the caller for longer
// than the lock timeout.
func (s *Store) Record(obs *Observation) error {
	if obs == nil {
		return nil
	}

	if obs.Time.IsZero() {
		obs.Time = s.now()
	}

	return s.locked(func() error {
		salt, err := s.salt(true)
		if err != nil {
			return err
		}

		line, err := json.Marshal(build(obs, keyed(salt)))
		if err != nil {
			return errors.Wrap(err, "failed to encode metrics record")
		}

		return s.append(append(line, '\n'))
	})
}

// append writes one line and rotates the log once it passes the size cap.
func (s *Store) append(line []byte) error {
	file, err := os.OpenFile(filepath.Clean(s.path), os.O_WRONLY|os.O_APPEND|os.O_CREATE, fileMode)
	if err != nil {
		return errors.Wrap(err, "failed to open metrics log")
	}

	_, writeErr := file.Write(line)

	info, statErr := file.Stat()
	closeErr := file.Close()

	if err := errors.CombineErrors(writeErr, closeErr); err != nil {
		return errors.Wrap(err, "failed to write metrics log")
	}

	if statErr == nil && info.Size() >= s.maxSize {
		if err := os.Rename(s.path, s.path+backupSuffix); err != nil {
			return errors.Wrap(err, "failed to rotate metrics log")
		}
	}

	return nil
}

// Load returns the records at or after since, oldest first, from the backup
// and the active log, and how many lines could not be read. A line cut
// short by a crash, or written by a newer release, is skipped. Both files
// are opened under the lock, so a rotation cannot move records between
// them unseen; reading happens after the lock is released, so hooks are not
// kept waiting by a large report. Without the lock (a read-only state
// directory) the files are read as they are.
func (s *Store) Load(since time.Time) ([]Record, int, error) {
	var files []*os.File

	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()

	open := func() error {
		for _, path := range []string{s.path + backupSuffix, s.path} {
			file, err := os.Open(filepath.Clean(path))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			if err != nil {
				return errors.Wrap(err, "failed to open metrics log")
			}

			files = append(files, file)
		}

		return nil
	}

	if err := s.lockedFor(readLockTimeout, open); err != nil {
		for _, file := range files {
			_ = file.Close()
		}

		files = nil

		if err := open(); err != nil {
			return nil, 0, err
		}
	}

	var (
		records []Record
		skipped int
	)

	for _, file := range files {
		recs, bad, err := readLog(file, since)
		if err != nil {
			return nil, 0, err
		}

		records = append(records, recs...)
		skipped += bad
	}

	return records, skipped, nil
}

// Prune drops records older than the retention from both files and returns
// how many it dropped.
func (s *Store) Prune() (int, error) {
	cutoff := s.now().Add(-s.retention)
	dropped := 0

	err := s.locked(func() error {
		for _, path := range []string{s.path + backupSuffix, s.path} {
			n, err := pruneLog(path, cutoff)
			if err != nil {
				return err
			}

			dropped += n
		}

		return nil
	})

	return dropped, err
}

// Clear removes the logs and the salt, so later records cannot be linked to
// earlier ones.
func (s *Store) Clear() error {
	return s.locked(func() error {
		for _, path := range []string{s.path, s.path + backupSuffix, s.saltPath()} {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return errors.Wrap(err, "failed to remove metrics file")
			}
		}

		return nil
	})
}

// Probe checks that a hook could record: the directory exists or can be
// created, the lock can be taken, and the log opened for appending. It
// writes no record.
func (s *Store) Probe() error {
	return s.locked(func() error {
		file, err := os.OpenFile(
			filepath.Clean(s.path),
			os.O_WRONLY|os.O_APPEND|os.O_CREATE,
			fileMode,
		)
		if err != nil {
			return errors.Wrap(err, "failed to open metrics log")
		}

		return errors.Wrap(file.Close(), "failed to close metrics log")
	})
}

// Size returns the bytes kept in the active log and its backup.
func (s *Store) Size() int64 {
	var total int64

	for _, path := range []string{s.path, s.path + backupSuffix} {
		if info, err := os.Stat(path); err == nil {
			total += info.Size()
		}
	}

	return total
}

// MaxSize returns the most the active log and its backup can hold.
func (s *Store) MaxSize() int64 {
	return keptFiles * s.maxSize
}

// locked runs fn holding the log's file lock.
func (s *Store) locked(fn func() error) error {
	return s.lockedFor(s.lockTimeout, fn)
}

// lockedFor runs fn holding the log's file lock, waiting up to timeout.
func (s *Store) lockedFor(timeout time.Duration, fn func() error) (err error) {
	if err = os.MkdirAll(filepath.Dir(s.path), dirMode); err != nil {
		return errors.Wrap(err, "failed to create metrics directory")
	}

	lock, err := filelock.Acquire(s.path+lockSuffix, timeout)
	if err != nil {
		return errors.Wrap(err, "failed to lock metrics log")
	}

	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			err = errors.Wrap(releaseErr, "failed to unlock metrics log")
		}
	}()

	return fn()
}

func (s *Store) saltPath() string {
	return filepath.Join(filepath.Dir(s.path), saltFile)
}

// salt returns the key that hashes sessions and resources, creating it when
// create is set. Callers hold the lock.
func (s *Store) salt(create bool) ([]byte, error) {
	data, err := os.ReadFile(s.saltPath())
	if err == nil && len(data) >= saltBytes {
		return data, nil
	}

	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Wrap(err, "failed to read metrics salt")
	}

	if !create {
		return nil, nil
	}

	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Wrap(err, "failed to create metrics salt")
	}

	if err := writeAtomic(s.saltPath(), salt); err != nil {
		return nil, err
	}

	return salt, nil
}

// keyed returns a hasher whose keys only this salt reproduces.
func keyed(salt []byte) hasher {
	return func(parts ...string) string {
		mac := hmac.New(sha256.New, salt)
		mac.Write([]byte(strings.Join(parts, "\x00")))

		return hex.EncodeToString(mac.Sum(nil))[:keyChars]
	}
}

func readLog(file *os.File, since time.Time) ([]Record, int, error) {
	var (
		records []Record
		skipped int
	)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineBytes)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil || rec.Time.IsZero() {
			skipped++

			continue
		}

		if rec.Time.Before(since) {
			continue
		}

		records = append(records, rec)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, errors.Wrap(err, "failed to read metrics log")
	}

	return records, skipped, nil
}

// pruneLog rewrites path without records older than cutoff or lines that
// cannot be read.
func pruneLog(path string, cutoff time.Time) (int, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, errors.Wrap(err, "failed to read metrics log")
	}

	var (
		kept    bytes.Buffer
		dropped int
	)

	for line := range bytes.Lines(data) {
		var rec Record
		if json.Unmarshal(line, &rec) != nil || rec.Time.Before(cutoff) {
			if len(bytes.TrimSpace(line)) > 0 {
				dropped++
			}

			continue
		}

		kept.Write(line)
	}

	if dropped == 0 {
		return 0, nil
	}

	return dropped, writeAtomic(path, kept.Bytes())
}

// writeAtomic replaces path through a temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+tempPattern)
	if err != nil {
		return errors.Wrap(err, "failed to create metrics temp file")
	}

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()

	if err := errors.CombineErrors(writeErr, closeErr); err != nil {
		_ = os.Remove(tmp.Name())

		return errors.Wrap(err, "failed to write metrics temp file")
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())

		return errors.Wrap(err, "failed to replace metrics file")
	}

	return nil
}
