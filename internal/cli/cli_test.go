package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdl/internal/library"
	"mdl/internal/runner"
	"mdl/internal/tui"
)

// harness bundles an App with captured output and a temp library path.
type harness struct {
	app      *App
	out, err *bytes.Buffer
	fake     *runner.Fake
	dir      string
	libPath  string
	musicDir string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{out: &bytes.Buffer{}, err: &bytes.Buffer{}, fake: &runner.Fake{}, dir: dir}
	h.libPath = filepath.Join(dir, "cfg", "library.json")
	h.musicDir = filepath.Join(dir, "music")
	env := map[string]string{"MDL_LIBRARY": h.libPath, "MDL_MUSIC_DIR": h.musicDir}
	h.app = &App{
		Version:  "test",
		Stdout:   h.out,
		Stderr:   h.err,
		Getenv:   func(k string) string { return env[k] },
		Home:     dir,
		Runner:   h.fake,
		LookPath: func(n string) (string, error) { return "", errors.New("not found") },
		TUI:      func(*library.Library) (tui.Result, error) { return tui.Result{}, nil },
	}
	return h
}

func (h *harness) run(args ...string) int {
	h.out.Reset()
	h.err.Reset()
	h.fake.Calls = nil
	return h.app.Run(context.Background(), args)
}

func (h *harness) seed(t *testing.T) {
	t.Helper()
	l := library.New()
	for _, e := range []struct {
		artist, album string
		e             library.Entry
	}{
		{"Daft Punk", "Discovery", library.Entry{Spotify: "https://open.spotify.com/album/a", YouTube: "https://music.youtube.com/playlist?list=b"}},
		{"Daft Punk", "Homework", library.Entry{YouTube: "https://music.youtube.com/playlist?list=c"}},
		{"AC/DC", "Back in Black", library.Entry{Spotify: "https://open.spotify.com/album/d"}},
	} {
		if err := l.Add(e.artist, e.album, e.e); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Save(h.libPath); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) expect(t *testing.T, code int, args ...string) {
	t.Helper()
	if got := h.run(args...); got != code {
		t.Fatalf("mdl %s: exit %d, want %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), got, code, h.out, h.err)
	}
}

func TestHelpAndVersion(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{}, {"-h"}, {"--help"}, {"help"}, {"-n"}} {
		h.expect(t, 0, args...)
		if !strings.Contains(h.out.String(), "COMMANDS") || !strings.Contains(h.out.String(), "GLOBAL OPTIONS") {
			t.Errorf("mdl %v: not the full help:\n%s", args, h.out)
		}
	}
	for _, args := range [][]string{{"-V"}, {"--version"}, {"version"}, {"list", "-V"}} {
		h.expect(t, 0, args...)
		if h.out.String() != "mdl test\n" {
			t.Errorf("mdl %v printed %q", args, h.out)
		}
	}
	for _, args := range [][]string{{"download", "-h"}, {"download", "--help"}, {"-h", "download"}, {"help", "download"}, {"dl", "--help"}} {
		h.expect(t, 0, args...)
		if !strings.HasPrefix(h.out.String(), "mdl download") || !strings.Contains(h.out.String(), "--yt-dlp") {
			t.Errorf("mdl %v: not download help:\n%s", args, h.out)
		}
	}
	h.expect(t, 2, "help", "bogus")
	h.expect(t, 0, "help", "--help")
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"frobnicate"},
		{"--bogus"},
		{"list", "--bogus"},
		{"download", "-sy"},
		{"download", "-s", "--yt-dlp"},
		{"tui", "-sy"},
		{"add"},
		{"add", "-a", "X", "-A", "Y"},
		{"add", "-a", "X", "-A", "Y", "-u", "https://u.example", "stray"},
		{"import"},
		{"import", "a", "b"},
		{"beet", "a", "b"},
	} {
		if code := h.run(args...); code != 2 {
			t.Errorf("mdl %v: exit %d, want 2 (stderr %q)", args, code, h.err)
		}
		if !strings.Contains(h.err.String(), "--help") {
			t.Errorf("mdl %v: usage errors should point at --help: %q", args, h.err)
		}
	}
}

func TestAddAndList(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "add", "-a", "Daft Punk", "-A", "Discovery", "-u", "https://open.spotify.com/album/a?x=1&y=2", "-u", "https://music.youtube.com/playlist?list=b")
	h.expect(t, 0, "add", "--artist=daft punk", "--album", "Homework", "--url", "https://open.spotify.com/album/c")
	h.expect(t, 0, "add", "-a", "Abba", "-A", "Arrival", "-u", "https://youtu.be/x")

	l, err := library.Load(h.libPath)
	if err != nil || l.NumArtists() != 2 || l.NumAlbums() != 3 {
		t.Fatalf("library = %v, %v", l, err)
	}
	h.expect(t, 0, "list")
	for _, want := range []string{"Abba (1)", "Daft Punk (2)", "├─ Discovery  [spotify, youtube]", "└─ Homework  [spotify]"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("list output missing %q:\n%s", want, h.out)
		}
	}
	if strings.Contains(h.out.String(), "https://") {
		t.Error("list without -u must not print URLs")
	}
	h.expect(t, 0, "ls", "-u", "-a", "DAFT PUNK")
	if !strings.Contains(h.out.String(), "spotify https://open.spotify.com/album/a?x=1&y=2") || strings.Contains(h.out.String(), "Abba") {
		t.Errorf("filtered list wrong:\n%s", h.out)
	}
	h.expect(t, 1, "list", "-a", "nobody")
}

func TestAddErrors(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 1, "add", "-a", "A", "-A", "B", "-u", "https://example.com/x")
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("a failed add must not create the library")
	}
	h.expect(t, 0, "-n", "add", "-a", "A", "-A", "B", "-u", "https://youtu.be/x")
	if _, err := os.Stat(h.libPath); err == nil || !strings.Contains(h.out.String(), "would add") {
		t.Errorf("dry-run add should not write (out %q)", h.out)
	}
	// Invalid existing library.
	_ = os.MkdirAll(filepath.Dir(h.libPath), 0o755)
	_ = os.WriteFile(h.libPath, []byte("{broken"), 0o644)
	h.expect(t, 1, "add", "-a", "A", "-A", "B", "-u", "https://youtu.be/x")
	// Library path that cannot be written.
	h.app.Getenv = func(k string) string { return "" }
	h.expect(t, 1, "-L", filepath.Join(h.libPath, "sub", "x.json"), "add", "-a", "A", "-A", "B", "-u", "https://youtu.be/x")
}

func TestListMissingAndEmpty(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 1, "list")
	if !strings.Contains(h.err.String(), "mdl add") {
		t.Errorf("missing-library message should suggest `mdl add`: %q", h.err)
	}
	_ = os.MkdirAll(filepath.Dir(h.libPath), 0o755)
	_ = os.WriteFile(h.libPath, []byte("{}"), 0o644)
	h.expect(t, 0, "list")
	if !strings.Contains(h.out.String(), "empty") {
		t.Errorf("out = %q", h.out)
	}
	_ = os.WriteFile(h.libPath, []byte(`{"A":{"B":"https://x"}}`), 0o644)
	h.expect(t, 1, "list")
	if !strings.Contains(h.err.String(), "mdl import") {
		t.Errorf("old-format error should suggest `mdl import`: %q", h.err)
	}
}

func TestDownloadLibraryLayoutAndBeet(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.expect(t, 0, "download", "-b")

	var dirs []string
	for _, c := range h.fake.Calls {
		if c.Name != "beet" {
			dirs = append(dirs, c.Dir)
		}
	}
	want := []string{
		filepath.Join(h.musicDir, "AC_DC", "Back in Black"),
		filepath.Join(h.musicDir, "Daft Punk", "Discovery"),
		filepath.Join(h.musicDir, "Daft Punk", "Homework"),
	}
	if strings.Join(dirs, "|") != strings.Join(want, "|") {
		t.Errorf("download dirs = %v, want %v", dirs, want)
	}
	// beet runs once per downloaded directory, after all downloads.
	if n := len(h.fake.Calls); n != 6 {
		t.Fatalf("calls = %d, want 6: %+v", n, h.fake.Calls)
	}
	for i, d := range want {
		c := h.fake.Calls[3+i]
		if c.Name != "beet" || c.Args[len(c.Args)-1] != d {
			t.Errorf("beet call %d = %+v, want dir %s", i, c, d)
		}
	}
}

func TestDownloadFilters(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.expect(t, 0, "dl", "-a", "daft punk", "-A", "homework")
	if len(h.fake.Calls) != 1 || h.fake.Calls[0].Name != "yt-dlp" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	h.expect(t, 0, "get", "-s", "-a", "Daft Punk", "-A", "Discovery")
	if h.fake.Calls[0].Name != "spotdl" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	h.expect(t, 0, "download", "-y", "-A", "Discovery")
	if h.fake.Calls[0].Name != "yt-dlp" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	h.expect(t, 1, "download", "-a", "Nobody")
	h.expect(t, 1, "download", "-a", "Daft Punk", "-A", "nope")
}

func TestDownloadReportsFailuresButContinues(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.fake.Err = func(c runner.Call) error {
		if strings.Contains(c.Dir, "Discovery") {
			return errors.New("network down")
		}
		return nil
	}
	h.expect(t, 1, "download", "-b")
	if !strings.Contains(h.err.String(), "Daft Punk / Discovery: network down") {
		t.Errorf("stderr = %q", h.err)
	}
	downloads, beets := 0, 0
	for _, c := range h.fake.Calls {
		if c.Name == "beet" {
			beets++
		} else {
			downloads++
		}
	}
	if downloads != 3 || beets != 2 {
		t.Errorf("downloads=%d beets=%d, want 3 and 2 (failed album is not beeted)", downloads, beets)
	}
	// A forced tool missing a link is a per-album failure, not a crash.
	h.fake.Err = nil
	h.expect(t, 1, "download", "-s")
	if !strings.Contains(h.err.String(), "no Spotify link") {
		t.Errorf("stderr = %q", h.err)
	}
}

func TestDownloadBeetFailure(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.fake.Err = func(c runner.Call) error {
		if c.Name == "beet" {
			return errors.New("beet blew up")
		}
		return nil
	}
	h.expect(t, 1, "download", "-b", "-a", "AC/DC")
}

func TestDownloadURLs(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "download", "-b", "https://open.spotify.com/album/a", "https://music.youtube.com/playlist?list=x")
	got := []string{}
	for _, c := range h.fake.Calls {
		got = append(got, c.Name)
	}
	if strings.Join(got, ",") != "spotdl,yt-dlp,beet" {
		t.Errorf("calls = %v", got)
	}
	// MDL_MUSIC_DIR is set in the harness but must not redirect ad-hoc URLs:
	// they go to the current directory.
	if h.fake.Calls[0].Dir != "." || h.fake.Calls[2].Args[2] != "." {
		t.Errorf("URLs should download into the current directory: %+v", h.fake.Calls)
	}
	// An explicit -m wins.
	custom := filepath.Join(h.dir, "elsewhere")
	h.expect(t, 0, "download", "-m", custom, "https://open.spotify.com/album/a")
	if h.fake.Calls[0].Dir != custom {
		t.Errorf("-m should apply to URLs: %+v", h.fake.Calls[0])
	}
	h.fake.Err = func(runner.Call) error { return errors.New("fail") }
	h.expect(t, 1, "download", "-b", "https://open.spotify.com/album/a")
	for _, c := range h.fake.Calls {
		if c.Name == "beet" {
			t.Error("beet must not run when every download failed")
		}
	}
}

func TestDownloadMissingLibrary(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 1, "download")
}

func TestDryRun(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.app.Runner = &runner.Fake{Err: func(runner.Call) error { t.Error("dry run must not execute anything"); return nil }}
	h.expect(t, 0, "-n", "download", "-b")
	out := h.out.String()
	for _, want := range []string{"+ (cd ", "spotdl --bitrate 128k download", "yt-dlp ", "+ beet import -C"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(h.musicDir); err == nil {
		t.Error("dry run created the music directory")
	}
	h.expect(t, 0, "download", "--dry-run", "https://open.spotify.com/album/z")
}

func TestInterrupted(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.fake.Err = func(runner.Call) error { cancel(); return nil }
	if code := h.app.Run(ctx, []string{"download"}); code != 130 {
		t.Errorf("exit = %d, want 130", code)
	}
	if !strings.Contains(h.err.String(), "interrupted") {
		t.Errorf("stderr = %q", h.err)
	}
}

func TestBeetCommand(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "beet")
	if c := h.fake.Calls[0]; c.Name != "beet" || strings.Join(c.Args, " ") != "import -C ." {
		t.Errorf("call = %+v", c)
	}
	sub := filepath.Join(h.dir, "Some Album")
	_ = os.MkdirAll(sub, 0o755)
	h.expect(t, 0, "beet", sub)
	if c := h.fake.Calls[0]; c.Args[len(c.Args)-1] != sub {
		t.Errorf("call = %+v", c)
	}
	h.expect(t, 1, "beet", filepath.Join(h.dir, "does-not-exist"))
	h.expect(t, 1, "beet", filepath.Join(h.dir, "cfg")+"/../../nonexistent")
	h.fake.Err = func(runner.Call) error { return errors.New("beet failed") }
	h.expect(t, 1, "beet")
	h.fake.Err = nil
	h.expect(t, 0, "-n", "beet", "/definitely/not/here")
	if !strings.Contains(h.out.String(), "+ beet import -C /definitely/not/here") {
		t.Errorf("out = %q", h.out)
	}
}

func TestInstallCommand(t *testing.T) {
	h := newHarness(t)
	h.app.LookPath = func(n string) (string, error) {
		if n == "pipx" || n == "ffmpeg" || n == "node" || n == "beet" {
			return "/bin/" + n, nil
		}
		return "", errors.New("missing")
	}
	h.expect(t, 0, "install")
	if len(h.fake.Calls) != 2 || h.fake.Calls[0].Args[1] != "yt-dlp" || h.fake.Calls[1].Args[1] != "spotdl" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	h.expect(t, 0, "install", "-u", "beet")
	if len(h.fake.Calls) != 1 || h.fake.Calls[0].Args[0] != "upgrade" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	h.expect(t, 1, "install", "nonsense")
	h.expect(t, 1, "install", "--check")
	if !strings.Contains(h.out.String(), "ok       beet") || !strings.Contains(h.out.String(), "missing  yt-dlp") {
		t.Errorf("check output:\n%s", h.out)
	}
	if len(h.fake.Calls) != 0 {
		t.Error("--check must not install anything")
	}
	h.app.LookPath = func(n string) (string, error) { return "/bin/" + n, nil }
	h.expect(t, 0, "install", "-c")
	h.app.LookPath = func(n string) (string, error) { return "", errors.New("none") }
	h.expect(t, 1, "-n", "install", "spotdl") // no pipx/python available -> error is reported
}

func TestInstallDryRunPrintsCommands(t *testing.T) {
	h := newHarness(t)
	h.app.LookPath = func(n string) (string, error) {
		if n == "pipx" {
			return "/bin/pipx", nil
		}
		return "", errors.New("missing")
	}
	h.expect(t, 0, "-n", "install")
	for _, want := range []string{"+ pipx install beets", "+ pipx install yt-dlp", "+ pipx install spotdl"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, h.out)
		}
	}
}

func TestEditCommand(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "edit", "-e", "code -w")
	c := h.fake.Calls[0]
	if c.Name != "code" || strings.Join(c.Args, " ") != "-w "+h.libPath {
		t.Errorf("call = %+v", c)
	}
	if _, err := os.Stat(h.libPath); err != nil {
		t.Errorf("edit should create the library: %v", err)
	}
	if !strings.Contains(h.err.String(), "Format:") {
		t.Errorf("format hint missing: %q", h.err)
	}

	// $VISUAL beats $EDITOR; default is vi.
	h.app.Getenv = func(k string) string {
		return map[string]string{"MDL_LIBRARY": h.libPath, "VISUAL": "nano", "EDITOR": "ed"}[k]
	}
	h.expect(t, 0, "edit")
	if h.fake.Calls[0].Name != "nano" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
	h.app.Getenv = func(k string) string { return map[string]string{"MDL_LIBRARY": h.libPath, "EDITOR": "ed"}[k] }
	h.expect(t, 0, "edit")
	if h.fake.Calls[0].Name != "ed" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
	h.app.Getenv = func(k string) string { return map[string]string{"MDL_LIBRARY": h.libPath}[k] }
	h.expect(t, 0, "edit")
	if h.fake.Calls[0].Name != "vi" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}

	// An editor that leaves invalid JSON behind is reported.
	h.fake.Err = func(runner.Call) error { return os.WriteFile(h.libPath, []byte("{oops"), 0o644) }
	h.expect(t, 1, "edit")
	if !strings.Contains(h.err.String(), "no longer valid") {
		t.Errorf("stderr = %q", h.err)
	}
	// Editor failure.
	h.fake.Err = func(runner.Call) error { return errors.New("no such editor") }
	h.expect(t, 1, "edit")
	// Dry run does not create anything.
	h.fake.Err = nil
	fresh := filepath.Join(h.dir, "fresh.json")
	h.expect(t, 0, "-n", "-L", fresh, "edit")
	if _, err := os.Stat(fresh); err == nil {
		t.Error("dry-run edit created the file")
	}
	// Cannot create the library.
	blocker := filepath.Join(h.dir, "blocker")
	_ = os.WriteFile(blocker, nil, 0o644)
	h.expect(t, 1, "-L", filepath.Join(blocker, "x.json"), "edit")
}

func TestImportCommand(t *testing.T) {
	h := newHarness(t)
	single := filepath.Join(h.dir, "single.json")
	multi := filepath.Join(h.dir, "multi.json")
	_ = os.WriteFile(single, []byte(`{"Discovery":"https://open.spotify.com/album/a","Homework":"https://open.spotify.com/album/b"}`), 0o644)
	_ = os.WriteFile(multi, []byte(`{"Abba":{"Arrival":"https://youtu.be/x"}}`), 0o644)

	h.expect(t, 1, "import", single) // needs --artist
	if !strings.Contains(h.err.String(), "--artist") {
		t.Errorf("stderr = %q", h.err)
	}
	h.expect(t, 0, "-n", "import", single, "-a", "Daft Punk")
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("dry-run import wrote the library")
	}
	h.expect(t, 0, "import", single, "-a", "Daft Punk")
	h.expect(t, 0, "import", multi)
	l, _ := library.Load(h.libPath)
	if l.NumAlbums() != 3 || l.NumArtists() != 2 {
		t.Errorf("library = %d artists / %d albums", l.NumArtists(), l.NumAlbums())
	}
	h.expect(t, 1, "import", filepath.Join(h.dir, "missing.json"))
	_ = os.WriteFile(h.libPath, []byte("{bad"), 0o644)
	h.expect(t, 1, "import", multi)
}

func TestTUICommand(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 1, "tui") // no library
	_ = os.MkdirAll(filepath.Dir(h.libPath), 0o755)
	_ = os.WriteFile(h.libPath, []byte("{}"), 0o644)
	h.expect(t, 1, "tui") // empty library
	h.seed(t)

	// Quit without choosing anything.
	h.expect(t, 0, "tui")
	if len(h.fake.Calls) != 0 {
		t.Error("quitting the TUI must not download")
	}
	// Download a single album, then beet it.
	h.app.TUI = func(l *library.Library) (tui.Result, error) {
		if l.NumAlbums() != 3 {
			t.Errorf("TUI got a library with %d albums", l.NumAlbums())
		}
		return tui.Result{Action: tui.ActionDownload, Artist: "Daft Punk", Album: "Homework"}, nil
	}
	h.expect(t, 0, "tui", "-b")
	if len(h.fake.Calls) != 2 || h.fake.Calls[0].Name != "yt-dlp" || h.fake.Calls[1].Name != "beet" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	// Whole artist.
	h.app.TUI = func(*library.Library) (tui.Result, error) {
		return tui.Result{Action: tui.ActionDownload, Artist: "Daft Punk"}, nil
	}
	h.expect(t, 0, "tui", "-y")
	if len(h.fake.Calls) != 2 {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	// Stale selection and TUI errors.
	h.app.TUI = func(*library.Library) (tui.Result, error) {
		return tui.Result{Action: tui.ActionDownload, Artist: "Gone"}, nil
	}
	h.expect(t, 1, "tui")
	h.app.TUI = func(*library.Library) (tui.Result, error) { return tui.Result{}, errors.New("no tty") }
	h.expect(t, 1, "tui")
	if !strings.Contains(h.err.String(), "no tty") {
		t.Errorf("stderr = %q", h.err)
	}
}

func TestDefaultsFromEnvironment(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	// -L and -m override the environment.
	other := filepath.Join(h.dir, "other.json")
	l := library.New()
	_ = l.Add("Solo", "Single", library.Entry{Spotify: "https://open.spotify.com/album/s"})
	_ = l.Save(other)
	custom := filepath.Join(h.dir, "custom")
	h.expect(t, 0, "-L", other, "-m", custom, "download")
	if got := h.fake.Calls[0].Dir; got != filepath.Join(custom, "Solo", "Single") {
		t.Errorf("dir = %q", got)
	}
	// Without env, the music dir is "." and the library lives under home.
	h.app.Getenv = func(string) string { return "" }
	_ = library.New().Save(filepath.Join(h.dir, ".config", "mdl", "library.json"))
	h.expect(t, 0, "list")
	h.app.Home = ""
	h.expect(t, 1, "list") // ./library.json does not exist
}

func TestNewApp(t *testing.T) {
	a := NewApp("v1")
	if a.Version != "v1" || a.Runner == nil || a.LookPath == nil || a.TUI == nil || a.Getenv == nil {
		t.Errorf("NewApp not fully wired: %+v", a)
	}
}

func TestBareURLDownloadsToCurrentDirectory(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "https://open.spotify.com/album/a")
	if len(h.fake.Calls) != 1 {
		t.Fatalf("calls = %+v", h.fake.Calls)
	}
	c := h.fake.Calls[0]
	if c.Name != "spotdl" || c.Dir != "." || c.Args[len(c.Args)-1] != "https://open.spotify.com/album/a" {
		t.Errorf("call = %+v", c)
	}
	// YouTube links pick yt-dlp; several URLs work; no library is needed.
	h.expect(t, 0, "https://music.youtube.com/playlist?list=x", "HTTP://OPEN.SPOTIFY.COM/album/b")
	if len(h.fake.Calls) != 2 || h.fake.Calls[0].Name != "yt-dlp" || h.fake.Calls[1].Name != "spotdl" {
		t.Errorf("calls = %+v", h.fake.Calls)
	}
	// A bare URL does not need an existing library; it creates one.
	if _, err := os.Stat(h.libPath); err != nil {
		t.Errorf("a bare URL should be recorded in a new library: %v", err)
	}
}

func TestBareURLWithOptions(t *testing.T) {
	h := newHarness(t)
	const spot = "https://open.spotify.com/album/a"
	for _, args := range [][]string{{"-b", spot}, {spot, "-b"}, {"--beet", spot}} {
		h.expect(t, 0, args...)
		if n := len(h.fake.Calls); n != 2 || h.fake.Calls[0].Name != "spotdl" || h.fake.Calls[1].Name != "beet" {
			t.Errorf("mdl %v: want download then beet, calls = %+v", args, h.fake.Calls)
		}
	}
	h.expect(t, 0, "-n", "-b", spot) // dry run prints both commands, runs nothing
	if len(h.fake.Calls) != 0 || !strings.Contains(h.out.String(), "spotdl") || !strings.Contains(h.out.String(), "+ beet import -C .") {
		t.Errorf("dry run output:\n%s", h.out)
	}
	// Forcing: --youtube / --spotify (and the older --yt-dlp / --spotdl names).
	for flag, tool := range map[string]string{
		"--youtube": "yt-dlp", "-y": "yt-dlp", "--yt-dlp": "yt-dlp",
		"--spotify": "spotdl", "-s": "spotdl", "--spotdl": "spotdl",
	} {
		h.expect(t, 0, flag, spot)
		if h.fake.Calls[0].Name != tool {
			t.Errorf("mdl %s URL used %s, want %s", flag, h.fake.Calls[0].Name, tool)
		}
	}
	h.expect(t, 0, "-m", filepath.Join(h.dir, "out"), spot)
	if h.fake.Calls[0].Dir != filepath.Join(h.dir, "out") {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
	h.expect(t, 2, "-sy", spot)
	h.expect(t, 2, "--spotify", "--youtube", spot)
	h.expect(t, 2, "--spotdl", "--yt-dlp", spot) // aliases conflict too
	h.expect(t, 2, "--nope", spot)
	h.expect(t, 0, "-h", spot)
	if !strings.HasPrefix(h.out.String(), "mdl download") {
		t.Errorf("help for a bare URL should be the download help:\n%s", h.out)
	}
}

func TestBareArgumentsThatAreNotURLs(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 2, "open.spotify.com/album/a") // no scheme: not treated as a URL
	h.expect(t, 2, "-b", "nonsense")
	// Real command names still win, even after download-only options.
	h.expect(t, 2, "-s", "list") // -s is not a global option
	h.expect(t, 0, "list", "--help")
}

func TestForceOptionsOnLibraryDownloads(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.expect(t, 0, "download", "--youtube", "-a", "Daft Punk", "-A", "Discovery")
	if h.fake.Calls[0].Name != "yt-dlp" || !strings.Contains(strings.Join(h.fake.Calls[0].Args, " "), "playlist?list=b") {
		t.Errorf("--youtube should use the YouTube link: %+v", h.fake.Calls[0])
	}
	h.expect(t, 0, "download", "--spotify", "-a", "Daft Punk", "-A", "Discovery")
	if h.fake.Calls[0].Name != "spotdl" || !strings.Contains(strings.Join(h.fake.Calls[0].Args, " "), "album/a") {
		t.Errorf("--spotify should use the Spotify link: %+v", h.fake.Calls[0])
	}
	h.expect(t, 1, "download", "--youtube", "-a", "AC/DC") // only a Spotify link
	if !strings.Contains(h.err.String(), "--youtube") {
		t.Errorf("error should name the option: %q", h.err)
	}
}

func TestToLibraryDownloadsFileUnderArtistAndAlbum(t *testing.T) {
	h := newHarness(t)
	// Behave like the real downloader: create <Artist>/<Album> below the cwd.
	n := 0
	h.fake.Err = func(c runner.Call) error {
		dir := filepath.Join(c.Dir, "Daft Punk", "Discovery")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		n++ // a new file each call, like a real download
		return os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d.mp3", n)), nil, 0o644)
	}
	const url = "https://music.youtube.com/playlist?list=x"

	// Uses the music directory (MDL_MUSIC_DIR here), unlike a plain URL.
	h.expect(t, 0, "-t", "-b", url)
	if len(h.fake.Calls) != 2 {
		t.Fatalf("calls = %+v", h.fake.Calls)
	}
	dl, beet := h.fake.Calls[0], h.fake.Calls[1]
	if dl.Name != "yt-dlp" || dl.Dir != h.musicDir {
		t.Errorf("download call = %+v", dl)
	}
	if !strings.Contains(strings.Join(dl.Args, " "), "Unknown Artist") {
		t.Errorf("template should fall back to Unknown Artist: %v", dl.Args)
	}
	// beet is pointed at the new album folder only, never the whole library.
	if want := filepath.Join(h.musicDir, "Daft Punk", "Discovery"); beet.Name != "beet" || beet.Args[len(beet.Args)-1] != want {
		t.Errorf("beet call = %+v, want dir %s", beet, want)
	}

	// Long form, -m override, explicit artist/album, forced tool.
	custom := filepath.Join(h.dir, "elsewhere")
	h.expect(t, 0, "--to-library", "-m", custom, "-a", "AC/DC", "-A", "Back in Black", "--spotify", "https://x.example/a")
	if h.fake.Calls[0].Name != "spotdl" || h.fake.Calls[0].Dir != custom {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
	if got := strings.Join(h.fake.Calls[0].Args, " "); !strings.Contains(got, "AC_DC/Back in Black/{track-number}") {
		t.Errorf("args = %s", got)
	}

	// Two URLs that produce the same album are beeted once.
	_ = os.RemoveAll(h.musicDir)
	h.expect(t, 0, "-t", "-b", url, "https://youtu.be/y")
	beets := 0
	for _, c := range h.fake.Calls {
		if c.Name == "beet" {
			beets++
		}
	}
	if beets != 1 {
		t.Errorf("beet ran %d times, want 1 (calls %+v)", beets, h.fake.Calls)
	}
}

func TestToLibraryFailuresAndMisuse(t *testing.T) {
	h := newHarness(t)
	h.fake.Err = func(runner.Call) error { return errors.New("boom") }
	h.expect(t, 1, "-t", "-b", "https://music.youtube.com/playlist?list=x")
	for _, c := range h.fake.Calls {
		if c.Name == "beet" {
			t.Error("beet must not run after a failed download")
		}
	}
	h.fake.Err = nil
	_ = os.RemoveAll(h.musicDir)
	h.expect(t, 2, "-t")            // no URL
	h.expect(t, 2, "-t", "-a", "X") // still no URL
	h.expect(t, 0, "-n", "-t", "-a", "Daft Punk", "https://open.spotify.com/album/a")
	if !strings.Contains(h.out.String(), "Daft Punk/") || !strings.Contains(h.out.String(), "--output") {
		t.Errorf("dry-run should show the library template:\n%s", h.out)
	}
	if _, err := os.Stat(h.musicDir); err == nil {
		t.Error("dry run created the music dir")
	}
}

// chdir moves into h.dir/<rel> (created) for the duration of the test, so
// "the directory we're in" is predictable.
func (h *harness) chdir(t *testing.T, rel string) string {
	t.Helper()
	dir := filepath.Join(h.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func (h *harness) library(t *testing.T) *library.Library {
	t.Helper()
	l, err := library.Load(h.libPath)
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	return l
}

const (
	spotURL = "https://open.spotify.com/album/abc"
	ytURL   = "https://music.youtube.com/playlist?list=OLAK5"
)

func TestEveryURLIsRecordedUnderTheDirectoryNames(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Daft Punk/Discovery")

	h.expect(t, 0, spotURL)
	if !strings.Contains(h.out.String(), "Recorded Daft Punk / Discovery") {
		t.Errorf("out = %q", h.out)
	}
	h.expect(t, 0, ytURL) // same folder: the YouTube link joins the same album
	l := h.library(t)
	e, ok := l.Get("Daft Punk", "Discovery")
	if !ok || e.Spotify != spotURL || e.YouTube != ytURL || l.NumAlbums() != 1 {
		t.Errorf("entry = %+v ok=%v albums=%d", e, ok, l.NumAlbums())
	}
	// Recording needs no metadata lookup: the download call itself is unchanged.
	if h.fake.Calls[0].Name != "yt-dlp" || h.fake.Calls[0].Dir != "." {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
}

func TestRecordOverridesAndOptOut(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Some Artist/Some Album")

	h.expect(t, 0, "-a", "Daft Punk", "-A", "Homework", spotURL)
	if _, ok := h.library(t).Get("Daft Punk", "Homework"); !ok {
		t.Error("--artist/--album should name the entry")
	}
	// Only one given: the other still comes from the directories.
	h.expect(t, 0, "-A", "Only Album", ytURL)
	if _, ok := h.library(t).Get("Some Artist", "Only Album"); !ok {
		t.Errorf("artist should come from the parent directory: %v", h.library(t).Artists())
	}
	h.expect(t, 0, "-a", "Only Artist", spotURL+"2")
	if _, ok := h.library(t).Get("Only Artist", "Some Album"); !ok {
		t.Error("album should come from the current directory")
	}

	// --no-import / -N: download only, library untouched.
	before := h.library(t).NumAlbums()
	h.expect(t, 0, "--no-import", spotURL+"3")
	h.expect(t, 0, "-N", spotURL+"4")
	if h.library(t).NumAlbums() != before {
		t.Error("--no-import must not touch the library")
	}
	fresh := filepath.Join(h.dir, "fresh.json")
	h.expect(t, 0, "-N", "-L", fresh, spotURL)
	if _, err := os.Stat(fresh); err == nil {
		t.Error("--no-import must not create the library")
	}
}

func TestRecordOnlyAfterSuccessfulDownload(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "A/B")
	h.fake.Err = func(runner.Call) error { return errors.New("network down") }
	h.expect(t, 1, spotURL)
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("a failed download must not be recorded")
	}
	// Two URLs, the first fails: only the second is recorded.
	h.fake.Err = func(c runner.Call) error {
		if strings.Contains(strings.Join(c.Args, " "), "bad") {
			return errors.New("boom")
		}
		return nil
	}
	h.expect(t, 1, "https://open.spotify.com/album/bad", ytURL)
	e, _ := h.library(t).Get("A", "B")
	if e.Spotify != "" || e.YouTube != ytURL {
		t.Errorf("entry = %+v", e)
	}
}

func TestRecordWarnsAboutUnstorableAndReplacedLinks(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "A/B")
	// A forced tool lets any host through; it downloads but cannot be stored.
	h.expect(t, 0, "--youtube", "https://example.com/video")
	if !strings.Contains(h.err.String(), "not recorded in the library") {
		t.Errorf("stderr = %q", h.err)
	}
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("nothing should have been stored")
	}
	h.expect(t, 0, spotURL)
	h.expect(t, 0, "https://open.spotify.com/album/other")
	if !strings.Contains(h.err.String(), "replacing the existing link for A / B") {
		t.Errorf("stderr = %q", h.err)
	}
	if e, _ := h.library(t).Get("A", "B"); e.Spotify != "https://open.spotify.com/album/other" {
		t.Errorf("entry = %+v", e)
	}
	h.expect(t, 0, "https://open.spotify.com/album/other") // same link again: no warning
	if strings.Contains(h.err.String(), "replacing") {
		t.Errorf("unexpected warning: %q", h.err)
	}
}

func TestRecordWithCorruptLibraryKeepsDownloadButFails(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "A/B")
	_ = os.MkdirAll(filepath.Dir(h.libPath), 0o755)
	_ = os.WriteFile(h.libPath, []byte("{broken"), 0o644)
	h.expect(t, 1, spotURL)
	if len(h.fake.Calls) != 1 {
		t.Errorf("the download should still have run: %+v", h.fake.Calls)
	}
	if raw, _ := os.ReadFile(h.libPath); string(raw) != "{broken" {
		t.Errorf("a corrupt library must be left untouched, got %q", raw)
	}
	// Unwritable library location.
	h.expect(t, 1, "-L", filepath.Join(h.libPath, "x", "lib.json"), spotURL)
}

func TestRecordDryRun(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "A/B")
	h.expect(t, 0, "-n", spotURL)
	if !strings.Contains(h.out.String(), "would record A / B in "+h.libPath) {
		t.Errorf("out = %q", h.out)
	}
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("dry run must not write the library")
	}
}

func TestToLibraryRecordsTheFolderThatWasFilled(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Some/Where")
	fill := func(paths ...string) {
		n := 0
		h.fake.Err = func(c runner.Call) error {
			for _, p := range paths {
				dir := filepath.Join(c.Dir, filepath.FromSlash(p))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				n++
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.mp3", n)), nil, 0o644); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Exactly one folder produced: that is where it is recorded.
	fill("Daft Punk/Discovery")
	h.expect(t, 0, "-t", ytURL)
	if e, ok := h.library(t).Get("Daft Punk", "Discovery"); !ok || e.YouTube != ytURL {
		t.Errorf("entry = %+v ok=%v", e, ok)
	}
	// --artist wins; the album still comes from the folder.
	_ = os.RemoveAll(h.musicDir)
	fill("Unknown Artist/Mixtape")
	h.expect(t, 0, "-t", "-a", "Real Artist", spotURL)
	if _, ok := h.library(t).Get("Real Artist", "Mixtape"); !ok {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	// Nothing detectable (or several folders): fall back to the directories.
	_ = os.RemoveAll(h.musicDir)
	h.fake.Err = nil
	h.expect(t, 0, "-t", spotURL+"x")
	if e, ok := h.library(t).Get("Some", "Where"); !ok || e.Spotify != spotURL+"x" {
		t.Errorf("fallback entry = %+v ok=%v", e, ok)
	}
	_ = os.RemoveAll(h.musicDir)
	fill("One/A", "Two/B")
	h.expect(t, 0, "-t", ytURL+"y")
	if e, _ := h.library(t).Get("Some", "Where"); e.YouTube != ytURL+"y" {
		t.Errorf("several folders should fall back: %+v", e)
	}
}

func TestRecordNames(t *testing.T) {
	wd, _ := os.Getwd()
	wantArtist, wantAlbum := filepath.Base(filepath.Dir(wd)), filepath.Base(wd)
	a, b := recordNames("", "", nil)
	if a != wantArtist || b != wantAlbum {
		t.Errorf("directory fallback = %q / %q, want %q / %q", a, b, wantArtist, wantAlbum)
	}
	if a, b := recordNames("A", "B", []string{"/m/X/Y"}); a != "A" || b != "B" {
		t.Errorf("flags win: %q / %q", a, b)
	}
	if a, b := recordNames("", "", []string{"/m/X/Y"}); a != "X" || b != "Y" {
		t.Errorf("landed folder: %q / %q", a, b)
	}
	if a, b := recordNames("", "B", []string{"/m/X/Y", "/m/P/Q"}); a != wantArtist || b != "B" {
		t.Errorf("ambiguous folders fall back per field: %q / %q", a, b)
	}
}
