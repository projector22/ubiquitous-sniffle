package deps

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"mdl/internal/runner"
)

// have returns a LookPath that only knows the given executables.
func have(names ...string) func(string) (string, error) {
	return func(n string) (string, error) {
		for _, h := range names {
			if h == n {
				return "/bin/" + n, nil
			}
		}
		return "", errors.New("not found")
	}
}

func newInstaller(look func(string) (string, error)) (*Installer, *runner.Fake, *bytes.Buffer) {
	f := &runner.Fake{}
	var out bytes.Buffer
	return &Installer{Runner: f, LookPath: look, Out: &out}, f, &out
}

func TestLookupAndNames(t *testing.T) {
	if d, ok := Lookup("beets"); !ok || d.Binary != "beet" {
		t.Errorf("Lookup(beets) = %+v, %v", d, ok)
	}
	if d, ok := Lookup("spotdl"); !ok || d.Package != "spotdl" {
		t.Errorf("Lookup(spotdl) = %+v, %v", d, ok)
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("expected miss")
	}
	if got := strings.Join(Names(), ","); got != "beet,yt-dlp,spotdl" {
		t.Errorf("Names = %s", got)
	}
}

func TestCheck(t *testing.T) {
	i, _, _ := newInstaller(have("beet", "ffmpeg"))
	st := i.Check(append(append([]Dep{}, Managed...), Recommended...))
	found := map[string]bool{}
	for _, s := range st {
		found[s.Binary] = s.Found
	}
	if !found["beet"] || !found["ffmpeg"] || found["yt-dlp"] || found["spotdl"] || found["node"] {
		t.Errorf("found = %v", found)
	}
	if st[0].Path != "/bin/beet" {
		t.Errorf("path = %q", st[0].Path)
	}
}

func TestInstallAllWithPipx(t *testing.T) {
	i, f, out := newInstaller(have("pipx", "ffmpeg", "node"))
	if err := i.Install(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range f.Calls {
		got = append(got, c.Name+" "+strings.Join(c.Args, " "))
	}
	want := "pipx install beets|pipx install yt-dlp|pipx install spotdl"
	if strings.Join(got, "|") != want {
		t.Errorf("calls = %v", got)
	}
	// Binaries still not on PATH after install -> hint printed.
	if !strings.Contains(out.String(), "pipx ensurepath") {
		t.Errorf("expected PATH hint, got:\n%s", out.String())
	}
}

func TestInstallSkipsInstalledUnlessUpgrade(t *testing.T) {
	i, f, out := newInstaller(have("pipx", "yt-dlp", "ffmpeg", "node"))
	if err := i.Install(context.Background(), []string{"yt-dlp"}, false); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 0 || !strings.Contains(out.String(), "already installed") {
		t.Errorf("calls=%v out=%q", f.Calls, out.String())
	}
	if err := i.Install(context.Background(), []string{"yt-dlp"}, true); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 1 || strings.Join(f.Calls[0].Args, " ") != "upgrade yt-dlp" {
		t.Errorf("calls = %+v", f.Calls)
	}
}

func TestInstallWithPip(t *testing.T) {
	i, f, _ := newInstaller(have("python3", "ffmpeg", "node"))
	if err := i.Install(context.Background(), []string{"spotdl"}, false); err != nil {
		t.Fatal(err)
	}
	c := f.Calls[0]
	if c.Name != "python3" || strings.Join(c.Args, " ") != "-m pip install --user spotdl" {
		t.Errorf("call = %+v", c)
	}
	// Upgrade adds --upgrade; falls back to `python`.
	i, f, _ = newInstaller(have("python", "spotdl", "ffmpeg", "node"))
	_ = i.Install(context.Background(), []string{"spotdl"}, true)
	if c := f.Calls[0]; c.Name != "python" || !strings.Contains(strings.Join(c.Args, " "), "--upgrade") {
		t.Errorf("call = %+v", c)
	}
}

func TestInstallErrors(t *testing.T) {
	ctx := context.Background()

	i, _, _ := newInstaller(have())
	err := i.Install(ctx, nil, false)
	if err == nil || !strings.Contains(err.Error(), "neither pipx nor python") {
		t.Errorf("no installer: %v", err)
	}

	i, _, _ = newInstaller(have("pipx"))
	if err := i.Install(ctx, []string{"nonsense"}, false); err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Errorf("unknown dep: %v", err)
	}

	// One failing install must not stop the others.
	i, f, _ := newInstaller(have("pipx"))
	f.Err = func(c runner.Call) error {
		if strings.Contains(strings.Join(c.Args, " "), "yt-dlp") {
			return errors.New("pipx exploded")
		}
		return nil
	}
	err = i.Install(ctx, nil, false)
	if err == nil || !strings.Contains(err.Error(), "yt-dlp") || len(f.Calls) != 3 {
		t.Errorf("err=%v calls=%d", err, len(f.Calls))
	}
}

func TestInstallWarnsAboutRecommended(t *testing.T) {
	i, _, out := newInstaller(have("pipx", "beet", "yt-dlp", "spotdl"))
	if err := i.Install(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ffmpeg not found") {
		t.Errorf("expected ffmpeg warning:\n%s", out.String())
	}
	if strings.Contains(out.String(), "ensurepath") {
		t.Errorf("no PATH hint expected when everything is found:\n%s", out.String())
	}
}

func TestDryRunSkipsPostChecks(t *testing.T) {
	i, _, out := newInstaller(have("pipx"))
	i.DryRun = true
	if err := i.Install(context.Background(), []string{"beet"}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "warning") || strings.Contains(out.String(), "note") {
		t.Errorf("dry run should not print post-install checks:\n%s", out.String())
	}
}
