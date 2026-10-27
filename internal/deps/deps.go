// Package deps checks for, and installs, the external tools mdl drives:
// beets, yt-dlp and spotdl.
package deps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/projector22/ubiquitous-sniffle/internal/runner"
)

// Dep describes one external tool.
type Dep struct {
	Binary  string // executable name looked up on PATH
	Package string // PyPI package that provides it ("" = cannot be pip-installed)
	Purpose string
}

// Managed are the tools `mdl install` can install via pipx or pip.
var Managed = []Dep{
	{Binary: "beet", Package: "beets", Purpose: "tags and organises your library (beets)"},
	{Binary: "yt-dlp", Package: "yt-dlp", Purpose: "downloads from YouTube / YouTube Music"},
	{Binary: "spotdl", Package: "spotdl", Purpose: "downloads Spotify tracks via YouTube"},
}

// Recommended tools are only checked and reported, never installed, because
// they need your system package manager (and often root).
var Recommended = []Dep{
	{Binary: "ffmpeg", Purpose: "audio conversion; required by yt-dlp and spotdl"},
	{Binary: "node", Purpose: "JavaScript runtime used by yt-dlp (--js-runtimes node)"},
}

// Lookup finds a managed dependency by binary or package name.
func Lookup(name string) (Dep, bool) {
	for _, d := range Managed {
		if d.Binary == name || d.Package == name {
			return d, true
		}
	}
	return Dep{}, false
}

// Status is the result of checking for one tool.
type Status struct {
	Dep
	Path  string
	Found bool
}

// Installer installs tools using pipx when available, else `python -m pip`.
type Installer struct {
	Runner   runner.Runner
	LookPath func(string) (string, error)
	Out      io.Writer
	DryRun   bool
}

func (i *Installer) logf(format string, a ...any) { fmt.Fprintf(i.Out, format+"\n", a...) }

// Check reports whether each of ds is on PATH.
func (i *Installer) Check(ds []Dep) []Status {
	out := make([]Status, 0, len(ds))
	for _, d := range ds {
		p, err := i.LookPath(d.Binary)
		out = append(out, Status{Dep: d, Path: p, Found: err == nil})
	}
	return out
}

// plan builds the command that installs (or upgrades) d.
func (i *Installer) plan(d Dep, installed, upgrade bool) (string, []string, error) {
	if _, err := i.LookPath("pipx"); err == nil {
		if installed {
			return "pipx", []string{"upgrade", d.Package}, nil
		}
		return "pipx", []string{"install", d.Package}, nil
	}
	for _, py := range []string{"python3", "python"} {
		if _, err := i.LookPath(py); err == nil {
			args := []string{"-m", "pip", "install", "--user"}
			if installed || upgrade {
				args = append(args, "--upgrade")
			}
			return py, append(args, d.Package), nil
		}
	}
	return "", nil, errors.New("neither pipx nor python was found; install pipx (https://pipx.pypa.io) or Python 3 first")
}

// Install installs the named tools (binary or package names); no names means
// all Managed tools. Tools already present are skipped unless upgrade is set.
// Failures do not stop the remaining installs; they are joined in the result.
func (i *Installer) Install(ctx context.Context, names []string, upgrade bool) error {
	var targets []Dep
	if len(names) == 0 {
		targets = Managed
	}
	for _, n := range names {
		d, ok := Lookup(n)
		if !ok {
			return fmt.Errorf("unknown dependency %q (known: %s)", n, strings.Join(Names(), ", "))
		}
		targets = append(targets, d)
	}

	var errs []error
	for _, d := range targets {
		_, lookErr := i.LookPath(d.Binary)
		installed := lookErr == nil
		if installed && !upgrade {
			i.logf("ok       %s already installed", d.Binary)
			continue
		}
		name, args, err := i.plan(d, installed, upgrade)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Binary, err))
			continue
		}
		i.logf("install  %s", d.Binary)
		if err := i.Runner.Run(ctx, "", name, args...); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Binary, err))
		}
	}

	if !i.DryRun {
		i.reportMissing(targets)
	}
	return errors.Join(errs...)
}

// reportMissing warns about tools still not on PATH and about missing
// recommended tools.
func (i *Installer) reportMissing(targets []Dep) {
	for _, s := range i.Check(targets) {
		if !s.Found {
			i.logf("note     %s is still not on PATH; run `pipx ensurepath` or add ~/.local/bin to PATH and restart your shell", s.Binary)
			break
		}
	}
	for _, s := range i.Check(Recommended) {
		if !s.Found {
			i.logf("warning  %s not found: %s. Install it with your system package manager (e.g. `brew install %s` or `apt install %s`)",
				s.Binary, s.Purpose, s.Binary, s.Binary)
		}
	}
}

// Names lists the binary names of every managed dependency.
func Names() []string {
	out := make([]string, len(Managed))
	for i, d := range Managed {
		out[i] = d.Binary
	}
	return out
}
