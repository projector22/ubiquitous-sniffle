package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesParentsAndReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "file.txt")
	write := func(s string) error {
		return Write(path, 0o644, func(w io.Writer) error { _, err := io.WriteString(w, s); return err })
	}
	if err := write("one"); err != nil {
		t.Fatal(err)
	}
	if err := write("two"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Errorf("content = %q", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", st.Mode())
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestWriteFailuresLeaveTheOriginalAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := Write(path, 0o644, func(io.Writer) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "original" {
		t.Errorf("a failed write must not touch the file: %q", got)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
	// Parent that cannot be created.
	if err := Write(filepath.Join(path, "x"), 0o644, func(io.Writer) error { return nil }); err == nil {
		t.Error("expected an error when the parent is a file")
	}
}
