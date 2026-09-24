package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// CheckWritable reports the first directory or file under logsRoot that the
// current process cannot write to, or nil if every entry is writable.
//
// It exists to fail fast on a PVC whose existing contents were written by an
// older image that ran as root, when the storage class ignores fsGroup (e.g.
// local-path). Without the check the server starts, reports ready, and then
// silently fails to write logs or apply retention for any pod it has
// collected before.
func CheckWritable(logsRoot string) error {
	return filepath.WalkDir(logsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		if err := unix.Access(path, unix.W_OK); err != nil {
			return fmt.Errorf("%s is not writable by uid %d (gid %d): %w", path, os.Geteuid(), os.Getegid(), err)
		}
		return nil
	})
}
