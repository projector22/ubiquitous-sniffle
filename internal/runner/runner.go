// Package runner abstracts running external programs so that the rest of mdl
// can be tested without spawning real processes.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner runs an external program, optionally inside dir.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
}

// Exec runs real processes, wiring them to the given streams.
type Exec struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Run implements Runner.
func (e Exec) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = e.Stdin, e.Stdout, e.Stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%s is not installed or not on PATH (try `mdl install`): %w", name, err)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Dry prints what would be run, shell-quoted, and does nothing else.
type Dry struct{ Out io.Writer }

// Run implements Runner.
func (d Dry) Run(_ context.Context, dir, name string, args ...string) error {
	_, err := fmt.Fprintln(d.Out, Format(dir, name, args...))
	return err
}

// Format renders a command line the way a shell user would type it.
func Format(dir, name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	for _, p := range append([]string{name}, args...) {
		parts = append(parts, Quote(p))
	}
	cmd := strings.Join(parts, " ")
	if dir != "" {
		return fmt.Sprintf("+ (cd %s && %s)", Quote(dir), cmd)
	}
	return "+ " + cmd
}

// Quote shell-quotes a single word, leaving simple words untouched.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if !isShellSafe(r) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// isShellSafe reports whether r needs no quoting in a shell word.
func isShellSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		strings.ContainsRune("_@%+=:,./-", r)
}

// Call records one invocation made through a Fake.
type Call struct {
	Dir, Name string
	Args      []string
}

// Fake records calls instead of running them. If Err is set it decides the
// result of each call. Intended for tests.
type Fake struct {
	Calls []Call
	Err   func(Call) error
}

// Run implements Runner.
func (f *Fake) Run(_ context.Context, dir, name string, args ...string) error {
	c := Call{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	f.Calls = append(f.Calls, c)
	if f.Err != nil {
		return f.Err(c)
	}
	return nil
}
