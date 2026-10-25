// Package atomicfile writes files so readers never see a half-written one.
package atomicfile

import (
	"io"
	"os"
	"path/filepath"
)

// Write creates path (and any missing parent directories) by writing to a
// temporary file in the same directory and renaming it into place. fill
// writes the content. The file ends up with permissions perm.
func Write(path string, perm os.FileMode, fill func(io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".mdl-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if err := fill(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
