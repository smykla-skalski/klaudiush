package hooksession

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

const tempSuffix = ".tmp"

// orphanedTempAge is how old a temp file must be before cleanup treats it as
// left behind by an interrupted writer rather than in progress.
const orphanedTempAge = time.Minute

// writeFileAtomic replaces path with data through a uniquely named temp file
// in the same directory, so writers never share a temp file and a reader or
// an interrupted writer only ever sees the old or the new content. Nothing
// is fsynced: the state is a cache of findings, an empty file reads as no
// state, and a flush on every hook would stall the hooks queued on the lock.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*"+tempSuffix)
	if err != nil {
		return errors.Wrap(err, "failed to create hook session temp file")
	}

	tmpPath := tmp.Name()

	if err := writeAndClose(tmp, data); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, "failed to replace hook session state")
	}

	return nil
}

func writeAndClose(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		_ = file.Close()

		return errors.Wrap(err, "failed to write hook session temp file")
	}

	if err := file.Close(); err != nil {
		return errors.Wrap(err, "failed to close hook session temp file")
	}

	return nil
}

// removeOrphanedTempFiles deletes temp files interrupted writers left next to
// the state file, including the fixed-name one older releases wrote. Callers
// hold the state lock, so no current writer owns them.
func (s *Store) removeOrphanedTempFiles() {
	dir := filepath.Dir(s.stateFile)
	base := filepath.Base(s.stateFile)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-orphanedTempAge)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			!strings.HasPrefix(name, base+".") ||
			!strings.HasSuffix(name, tempSuffix) {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}

		_ = os.Remove(filepath.Join(dir, name))
	}
}
