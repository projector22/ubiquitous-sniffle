package downloader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
