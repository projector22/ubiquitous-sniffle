package downloader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mdl/internal/library"
	"mdl/internal/runner"
)

func lib(t *testing.T) *library.Library {
	t.Helper()
	l := library.New()
	for _, a := range []struct {
		artist, album string
		e             library.Entry
	}{
		{"Daft Punk", "Discovery", library.Entry{Spotify: "https://open.spotify.com/album/a", YouTube: "https://music.youtube.com/playlist?list=b"}},
		{"Daft Punk", "Homework", library.Entry{YouTube: "https://music.youtube.com/playlist?list=c"}},
		{"AC/DC", "Back in Black", library.Entry{Spotify: "https://open.spotify.com/album/d"}},
	} {
		if err := l.Add(a.artist, a.album, a.e); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func TestToolForURL(t *testing.T) {
	tests := map[string]Tool{
		"https://open.spotify.com/album/x":          Spotdl,
		"https://music.youtube.com/playlist?list=x": YtDlp,
		"https://www.youtube.com/watch?v=x":         YtDlp,
		"https://youtu.be/x":                        YtDlp,
		"https://example.org/whatever":              Spotdl,
		"https://youtube.example.org/p":             YtDlp, // contains "youtube"
		"%%%":                                       Spotdl,
	}
	for in, want := range tests {
		if got := ToolForURL(in); got != want {
			t.Errorf("ToolForURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommand(t *testing.T) {
	name, args := Command(Spotdl, "u&v")
	if name != "spotdl" || strings.Join(args, " ") != "--bitrate 128k download u&v" {
		t.Errorf("spotdl: %s %v", name, args)
	}
	name, args = Command(YtDlp, "U")
	if name != "yt-dlp" || args[len(args)-1] != "U" {
		t.Errorf("yt-dlp: %s %v", name, args)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-x", "--audio-format mp3", "--embed-metadata", "--embed-thumbnail", "%(playlist_index)02d - %(artist)s - %(title)s.%(ext)s", "--js-runtimes node"} {
		if !strings.Contains(joined, want) {
			t.Errorf("yt-dlp args missing %q: %s", want, joined)
		}
	}
}

func TestSelectSource(t *testing.T) {
	both := library.Entry{Spotify: "s", YouTube: "y"}
	tests := []struct {
		name    string
		e       library.Entry
		forced  Tool
		tool    Tool
		url     string
		wantErr bool
	}{
		{"auto prefers spotify", both, Auto, Spotdl, "s", false},
		{"auto falls back to youtube", library.Entry{YouTube: "y"}, Auto, YtDlp, "y", false},
		{"forced spotdl", both, Spotdl, Spotdl, "s", false},
		{"forced yt-dlp", both, YtDlp, YtDlp, "y", false},
		{"forced spotdl missing", library.Entry{YouTube: "y"}, Spotdl, Auto, "", true},
		{"forced yt-dlp missing", library.Entry{Spotify: "s"}, YtDlp, Auto, "", true},
		{"no links", library.Entry{}, Auto, Auto, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, url, err := SelectSource(tc.e, tc.forced)
			if (err != nil) != tc.wantErr || tool != tc.tool || url != tc.url {
				t.Errorf("got %q %q %v", tool, url, err)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := map[string]string{
		"AC/DC":               "AC_DC",
		`a<b>c:d"e\f|g?h*i`:   "a_b_c_d_e_f_g_h_i",
		"  spaced  ":          "spaced",
		"trailing dots...":    "trailing dots",
		"..":                  "_",
		".":                   "_",
		"":                    "_",
		"   ":                 "_",
		"ctrl\x00\x1bchars\n": "ctrlchars",
		"日本語のアルバム":            "日本語のアルバム",
		"../../etc/passwd":    ".._.._etc_passwd",
	}
	for in, want := range tests {
		got := SanitizeName(in)
		if got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, `/\`) || got == "." || got == ".." {
			t.Errorf("SanitizeName(%q) = %q is not a safe path component", in, got)
		}
	}
}

func TestDirs(t *testing.T) {
	root := filepath.Join("music")
	if got, want := ArtistDir(root, "AC/DC"), filepath.Join("music", "AC_DC"); got != want {
		t.Errorf("ArtistDir = %q", got)
	}
	if got, want := AlbumDir(root, "AC/DC", "Back in Black"), filepath.Join("music", "AC_DC", "Back in Black"); got != want {
		t.Errorf("AlbumDir = %q", got)
	}
}

func TestPlan(t *testing.T) {
	l := lib(t)
	jobs, err := Plan(l, "", "")
	if err != nil || len(jobs) != 3 {
		t.Fatalf("all: %v, %v", jobs, err)
	}
	if jobs[0].Artist != "AC/DC" || jobs[1].Album != "Discovery" || jobs[2].Album != "Homework" {
		t.Errorf("unexpected order: %+v", jobs)
	}
	if jobs, err = Plan(l, "daft punk", ""); err != nil || len(jobs) != 2 || jobs[0].Artist != "Daft Punk" {
		t.Errorf("artist filter: %v, %v", jobs, err)
	}
	if jobs, err = Plan(l, "Daft Punk", "HOMEWORK"); err != nil || len(jobs) != 1 || jobs[0].Album != "Homework" {
		t.Errorf("artist+album: %v, %v", jobs, err)
	}
	if jobs, err = Plan(l, "", "back in black"); err != nil || len(jobs) != 1 || jobs[0].Artist != "AC/DC" {
		t.Errorf("album only: %v, %v", jobs, err)
	}
	if _, err = Plan(l, "Nobody", ""); err == nil || !strings.Contains(err.Error(), "artist") {
		t.Errorf("unknown artist: %v", err)
	}
	if _, err = Plan(l, "Daft Punk", "Nope"); err == nil || !strings.Contains(err.Error(), "album") {
		t.Errorf("unknown album: %v", err)
	}
	if _, err = Plan(library.New(), "", ""); err == nil {
		t.Error("empty library should be an error")
	}
}

func TestDownloadJobCreatesArtistAlbumLayout(t *testing.T) {
	root := t.TempDir()
	f := &runner.Fake{}
	d := &Downloader{Runner: f, Root: root}
	jobs, _ := Plan(lib(t), "", "")
	results := d.DownloadJobs(context.Background(), jobs, Auto)

	if len(results) != 3 || len(f.Calls) != 3 {
		t.Fatalf("results=%d calls=%d", len(results), len(f.Calls))
	}
	want := []string{
		filepath.Join(root, "AC_DC", "Back in Black"),
		filepath.Join(root, "Daft Punk", "Discovery"),
		filepath.Join(root, "Daft Punk", "Homework"),
	}
	for i, w := range want {
		if results[i].Err != nil || results[i].Dir != w || f.Calls[i].Dir != w {
			t.Errorf("job %d: dir=%q call dir=%q err=%v, want %q", i, results[i].Dir, f.Calls[i].Dir, results[i].Err, w)
		}
		if st, err := os.Stat(w); err != nil || !st.IsDir() {
			t.Errorf("directory %q was not created", w)
		}
	}
	// Both Daft Punk albums share one artist directory (the original bug nested them).
	if filepath.Dir(want[1]) != filepath.Dir(want[2]) {
		t.Error("albums of one artist must share the artist directory")
	}
	if f.Calls[0].Name != "spotdl" || f.Calls[1].Name != "spotdl" || f.Calls[2].Name != "yt-dlp" {
		t.Errorf("tools: %s %s %s", f.Calls[0].Name, f.Calls[1].Name, f.Calls[2].Name)
	}
}

func TestDownloadJobsContinuesAfterFailure(t *testing.T) {
	f := &runner.Fake{Err: func(c runner.Call) error {
		if strings.Contains(c.Dir, "Discovery") {
			return errors.New("network down")
		}
		return nil
	}}
	var out bytes.Buffer
	d := &Downloader{Runner: f, Root: t.TempDir(), Out: &out}
	jobs, _ := Plan(lib(t), "", "")
	results := d.DownloadJobs(context.Background(), jobs, Auto)
	if len(results) != 3 || results[1].Err == nil || results[0].Err != nil || results[2].Err != nil {
		t.Errorf("results = %+v", results)
	}
	if !strings.Contains(out.String(), "[2/3] Daft Punk / Discovery") {
		t.Errorf("progress output = %q", out.String())
	}
}

func TestDownloadJobsForcedToolMissingLink(t *testing.T) {
	f := &runner.Fake{}
	d := &Downloader{Runner: f, Root: t.TempDir()}
	jobs, _ := Plan(lib(t), "", "")
	results := d.DownloadJobs(context.Background(), jobs, Spotdl)
	if results[2].Err == nil { // Homework has only YouTube
		t.Error("expected failure for album without a Spotify link")
	}
	if len(f.Calls) != 2 {
		t.Errorf("calls = %d, want 2", len(f.Calls))
	}
}

func TestDownloadJobsStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &runner.Fake{Err: func(runner.Call) error { cancel(); return nil }}
	d := &Downloader{Runner: f, Root: t.TempDir()}
	jobs, _ := Plan(lib(t), "", "")
	if results := d.DownloadJobs(ctx, jobs, Auto); len(results) != 1 {
		t.Errorf("results = %d, want 1", len(results))
	}
}

func TestDryRunCreatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "music")
	d := &Downloader{Runner: &runner.Fake{}, Root: root, DryRun: true}
	jobs, _ := Plan(lib(t), "", "")
	d.DownloadJobs(context.Background(), jobs, Auto)
	if err := d.DownloadURL(context.Background(), "https://open.spotify.com/album/x", Auto); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("dry run must not create %s", root)
	}
}

func TestDownloadURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "m")
	f := &runner.Fake{}
	d := &Downloader{Runner: f, Root: root}
	ctx := context.Background()
	if err := d.DownloadURL(ctx, "https://music.youtube.com/playlist?list=x", Auto); err != nil {
		t.Fatal(err)
	}
	if err := d.DownloadURL(ctx, "https://music.youtube.com/playlist?list=x", Spotdl); err != nil {
		t.Fatal(err)
	}
	if f.Calls[0].Name != "yt-dlp" || f.Calls[1].Name != "spotdl" || f.Calls[0].Dir != root {
		t.Errorf("calls = %+v", f.Calls)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("root not created: %v", err)
	}
	// Root that cannot be created.
	blocker := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(blocker, nil, 0o644)
	bad := &Downloader{Runner: f, Root: filepath.Join(blocker, "sub")}
	if err := bad.DownloadURL(ctx, "https://open.spotify.com/a", Auto); err == nil {
		t.Error("expected mkdir error")
	}
	if _, err := bad.DownloadJob(ctx, Job{Artist: "a", Album: "b", Entry: library.Entry{Spotify: "s"}}, Auto); err == nil {
		t.Error("expected mkdir error from DownloadJob")
	}
}

func TestBeet(t *testing.T) {
	f := &runner.Fake{}
	if err := Beet(context.Background(), f, "/m/A/B"); err != nil {
		t.Fatal(err)
	}
	c := f.Calls[0]
	if c.Name != "beet" || strings.Join(c.Args, " ") != "import -C /m/A/B" {
		t.Errorf("call = %+v", c)
	}
}

func TestLibraryCommandYtDlp(t *testing.T) {
	name, args := LibraryCommand(YtDlp, "https://music.youtube.com/playlist?list=x", "", "")
	if name != "yt-dlp" || args[len(args)-1] != "https://music.youtube.com/playlist?list=x" {
		t.Fatalf("%s %v", name, args)
	}
	tmpl := argAfter(t, args, "-o")
	want := "%(album_artist,artist,creator|Unknown Artist)s/%(album,playlist_title|Unknown Album)s/%(playlist_index&{:02d} - |)s%(track,title)s.%(ext)s"
	if tmpl != want {
		t.Errorf("template = %q\nwant       %q", tmpl, want)
	}
	// Same audio options as a plain download.
	joined := strings.Join(args, " ")
	for _, w := range []string{"-x", "--audio-format mp3", "--embed-metadata", "--embed-thumbnail", "--js-runtimes node"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in %s", w, joined)
		}
	}
	// Explicit names replace the metadata fields and are sanitised / escaped.
	_, args = LibraryCommand(YtDlp, "u", "AC/DC", "100% Rock: Live")
	if got := argAfter(t, args, "-o"); !strings.HasPrefix(got, "AC_DC/100%% Rock_ Live/") {
		t.Errorf("template = %q", got)
	}
	// Only one of the two given: the other still comes from metadata.
	_, args = LibraryCommand(YtDlp, "u", "Daft Punk", "")
	if got := argAfter(t, args, "-o"); !strings.HasPrefix(got, "Daft Punk/%(album,playlist_title|Unknown Album)s/") {
		t.Errorf("template = %q", got)
	}
}

func TestLibraryCommandSpotdl(t *testing.T) {
	name, args := LibraryCommand(Spotdl, "https://open.spotify.com/album/a", "", "")
	if name != "spotdl" || args[len(args)-2] != "download" || args[len(args)-1] != "https://open.spotify.com/album/a" {
		t.Fatalf("%s %v", name, args)
	}
	if got, want := argAfter(t, args, "--output"), "{album-artist}/{album}/{track-number} - {title}.{output-ext}"; got != want {
		t.Errorf("template = %q, want %q", got, want)
	}
	if argAfter(t, args, "--bitrate") != "128k" {
		t.Error("bitrate lost")
	}
	_, args = LibraryCommand(Spotdl, "u", "AC/DC {x}", "Back: in Black")
	if got := argAfter(t, args, "--output"); !strings.HasPrefix(got, "AC_DC (x)/Back_ in Black/{track-number}") {
		t.Errorf("template = %q", got)
	}
}

func argAfter(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("%s not found in %v", flag, args)
	return ""
}

// fakeDownload returns a runner.Fake that behaves like a downloader: it
// creates the given <artist>/<album> folders (with a file) below the cwd.
func fakeDownload(paths ...[2]string) *runner.Fake {
	return &runner.Fake{Err: func(c runner.Call) error {
		for _, p := range paths {
			dir := filepath.Join(c.Dir, p[0], p[1])
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "01 - track.mp3"), []byte("x"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}}
}

func TestDownloadURLToLibraryReportsOnlyChangedAlbums(t *testing.T) {
	root := t.TempDir()
	// An existing, untouched album that must not be reported.
	old := filepath.Join(root, "Old Artist", "Old Album")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, past, past)

	f := fakeDownload([2]string{"Daft Punk", "Discovery"}, [2]string{"Unknown Artist", "Unknown Album"})
	var out bytes.Buffer
	d := &Downloader{Runner: f, Root: root, Out: &out}
	dirs, err := d.DownloadURLToLibrary(context.Background(), "https://music.youtube.com/playlist?list=x", Auto, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "Daft Punk", "Discovery"),
		filepath.Join(root, "Unknown Artist", "Unknown Album"),
	}
	if strings.Join(dirs, "|") != strings.Join(want, "|") {
		t.Errorf("dirs = %v, want %v", dirs, want)
	}
	if f.Calls[0].Dir != root || f.Calls[0].Name != "yt-dlp" {
		t.Errorf("call = %+v", f.Calls[0])
	}
	if !strings.Contains(out.String(), "into the library") {
		t.Errorf("output = %q", out.String())
	}

	// Adding files to an existing album counts as a change; others do not.
	_ = os.Chtimes(filepath.Join(root, "Daft Punk", "Discovery"), past, past)
	_ = os.Chtimes(filepath.Join(root, "Unknown Artist", "Unknown Album"), past, past)
	d.Runner = &runner.Fake{Err: func(c runner.Call) error {
		return os.WriteFile(filepath.Join(c.Dir, "Daft Punk", "Discovery", "02 - more.mp3"), nil, 0o644)
	}}
	dirs, err = d.DownloadURLToLibrary(context.Background(), "https://youtu.be/y", Auto, "", "")
	if err != nil || len(dirs) != 1 || dirs[0] != filepath.Join(root, "Daft Punk", "Discovery") {
		t.Errorf("dirs = %v, err = %v", dirs, err)
	}
}

func TestDownloadURLToLibraryOptionsAndErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "music")
	f := fakeDownload([2]string{"Daft Punk", "Discovery"})
	d := &Downloader{Runner: f, Root: root}
	ctx := context.Background()

	// Forced tool and explicit names reach the command line; Root is created.
	if _, err := d.DownloadURLToLibrary(ctx, "https://open.spotify.com/album/a", YtDlp, "Daft Punk", "Discovery"); err != nil {
		t.Fatal(err)
	}
	if f.Calls[0].Name != "yt-dlp" || !strings.HasPrefix(argAfter(t, f.Calls[0].Args, "-o"), "Daft Punk/Discovery/") {
		t.Errorf("call = %+v", f.Calls[0])
	}
	// A failing download returns the error (callers skip beet then).
	d.Runner = &runner.Fake{Err: func(runner.Call) error { return errors.New("boom") }}
	if _, err := d.DownloadURLToLibrary(ctx, "https://open.spotify.com/album/a", Auto, "", ""); err == nil {
		t.Error("expected the download error")
	}
	// Dry run creates nothing.
	dry := filepath.Join(t.TempDir(), "never")
	dd := &Downloader{Runner: &runner.Fake{}, Root: dry, DryRun: true}
	if dirs, err := dd.DownloadURLToLibrary(ctx, "https://open.spotify.com/album/a", Auto, "", ""); err != nil || len(dirs) != 0 {
		t.Errorf("dirs=%v err=%v", dirs, err)
	}
	if _, err := os.Stat(dry); !os.IsNotExist(err) {
		t.Error("dry run must not create the root")
	}
	// Root that cannot be created.
	blocker := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(blocker, nil, 0o644)
	bad := &Downloader{Runner: &runner.Fake{}, Root: filepath.Join(blocker, "x")}
	if _, err := bad.DownloadURLToLibrary(ctx, "https://open.spotify.com/album/a", Auto, "", ""); err == nil {
		t.Error("expected mkdir error")
	}
}

func TestSnapshotIgnoresFilesAndMissingRoot(t *testing.T) {
	if got := snapshotAlbumDirs(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Errorf("snapshot = %v", got)
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "stray.txt"), nil, 0o644)
	_ = os.MkdirAll(filepath.Join(root, "Artist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "Artist", "loose.mp3"), nil, 0o644)
	_ = os.MkdirAll(filepath.Join(root, "Artist", "Album", "deeper"), 0o755)
	got := snapshotAlbumDirs(root)
	if len(got) != 1 {
		t.Errorf("snapshot = %v, want only Artist/Album", got)
	}
}

func TestNamesFromDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Daft Punk", "Discovery")
	if artist, album := NamesFromDir(dir); artist != "Daft Punk" || album != "Discovery" {
		t.Errorf("got %q / %q", artist, album)
	}
	// Relative paths resolve against the working directory.
	wd, _ := os.Getwd()
	if artist, album := NamesFromDir("."); artist != filepath.Base(filepath.Dir(wd)) || album != filepath.Base(wd) {
		t.Errorf("'.' gave %q / %q", artist, album)
	}
	// A filesystem root has no usable names.
	if artist, album := NamesFromDir(string(filepath.Separator)); artist != UnknownArtist || album != UnknownAlbum {
		t.Errorf("root gave %q / %q", artist, album)
	}
	// A directory directly below the root has an album but no artist.
	top := filepath.Join(string(filepath.Separator), "OnlyAlbum")
	if artist, album := NamesFromDir(top); artist != UnknownArtist || album != "OnlyAlbum" {
		t.Errorf("top-level dir gave %q / %q", artist, album)
	}
}

func TestNameOfDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Daft Punk")
	if name, ok := NameOfDir(dir); !ok || name != "Daft Punk" {
		t.Errorf("got %q %v", name, ok)
	}
	wd, _ := os.Getwd()
	if name, ok := NameOfDir("."); !ok || name != filepath.Base(wd) {
		t.Errorf("'.' gave %q %v", name, ok)
	}
	if name, ok := NameOfDir(string(filepath.Separator)); ok || name != "" {
		t.Errorf("root gave %q %v", name, ok)
	}
}
