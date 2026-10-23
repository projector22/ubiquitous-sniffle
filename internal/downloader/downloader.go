// Package downloader turns library entries and ad-hoc URLs into runs of
// spotdl / yt-dlp, and lays the results out as <root>/<Artist>/<Album>/.
//
// Unlike the original shell script it never changes the process working
// directory: each external command is started inside its destination
// directory instead, so one album can never end up nested in another.
package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"mdl/internal/library"
	"mdl/internal/runner"
)

// Tool names an external downloader.
type Tool string

// Supported tools. Auto means "pick for me".
const (
	Auto   Tool = ""
	Spotdl Tool = "spotdl"
	YtDlp  Tool = "yt-dlp"
)

// ToolForURL picks a downloader from the URL's host: YouTube / YouTube Music
// links go to yt-dlp, everything else (Spotify) to spotdl.
func ToolForURL(raw string) Tool {
	if src, err := library.ClassifyURL(raw); err == nil && src == library.SourceYouTube {
		return YtDlp
	}
	if u, err := url.Parse(raw); err == nil && strings.Contains(strings.ToLower(u.Hostname()), "youtube") {
		return YtDlp
	}
	return Spotdl
}

// Command returns the program and arguments that download rawURL with t.
// Arguments are passed as an argv slice, never through a shell, so URLs with
// '&' or spaces are safe.
func Command(t Tool, rawURL string) (string, []string) {
	if t == YtDlp {
		return "yt-dlp", []string{
			"--no-warnings",
			"--progress",
			"-f", "bestaudio/best",
			"-x",
			"--audio-format", "mp3",
			"--audio-quality", "0",
			"--embed-metadata",
			"--embed-thumbnail",
			"-o", "%(playlist_index)02d - %(artist)s - %(title)s.%(ext)s",
			"--js-runtimes", "node",
			rawURL,
		}
	}
	return "spotdl", []string{"--bitrate", "128k", "download", rawURL}
}

// SelectSource chooses which of an entry's links to use. When forced is Auto,
// Spotify is preferred and YouTube is the fallback.
func SelectSource(e library.Entry, forced Tool) (Tool, string, error) {
	switch forced {
	case Spotdl:
		if e.Spotify == "" {
			return Auto, "", errors.New("no Spotify link (remove --spotify or add one)")
		}
		return Spotdl, e.Spotify, nil
	case YtDlp:
		if e.YouTube == "" {
			return Auto, "", errors.New("no YouTube link (remove --youtube or add one)")
		}
		return YtDlp, e.YouTube, nil
	}
	switch {
	case e.Spotify != "":
		return Spotdl, e.Spotify, nil
	case e.YouTube != "":
		return YtDlp, e.YouTube, nil
	}
	return Auto, "", errors.New("entry has no links")
}

// SanitizeName makes s safe to use as a single path component on every
// platform: path separators and characters Windows forbids become '_',
// control characters are dropped, and leading/trailing spaces and trailing
// dots are trimmed. The result is never empty and never "." or "..".
func SanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			continue
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimRight(strings.TrimSpace(b.String()), ". ")
	out = strings.TrimLeft(out, " ")
	if out == "" {
		return "_"
	}
	return out
}

// ArtistDir is <root>/<Artist>.
func ArtistDir(root, artist string) string {
	return filepath.Join(root, SanitizeName(artist))
}

// AlbumDir is <root>/<Artist>/<Album>.
func AlbumDir(root, artist, album string) string {
	return filepath.Join(ArtistDir(root, artist), SanitizeName(album))
}

// Job is one album to download.
type Job struct {
	Artist, Album string
	Entry         library.Entry
}

// Result is the outcome of one Job.
type Result struct {
	Job Job
	Dir string
	Err error
}

// Plan selects jobs from lib. Empty artist/album mean "any"; names are
// matched case-insensitively. It is an error if nothing matches.
func Plan(lib *library.Library, artist, album string) ([]Job, error) {
	if artist != "" {
		a, ok := lib.ResolveArtist(artist)
		if !ok {
			return nil, fmt.Errorf("artist %q not found in the library", artist)
		}
		artist = a
	}
	var jobs []Job
	for _, a := range lib.Artists() {
		if artist != "" && a != artist {
			continue
		}
		for _, al := range lib.Albums(a) {
			if album != "" && !strings.EqualFold(al, strings.TrimSpace(album)) {
				continue
			}
			e, _ := lib.Get(a, al)
			jobs = append(jobs, Job{Artist: a, Album: al, Entry: e})
		}
	}
	if len(jobs) == 0 {
		if album != "" {
			return nil, fmt.Errorf("album %q not found in the library", album)
		}
		return nil, errors.New("the library has no albums to download")
	}
	return jobs, nil
}

// Downloader runs download jobs below Root.
type Downloader struct {
	Runner runner.Runner
	Root   string    // top-level music directory
	Out    io.Writer // progress messages
	DryRun bool      // skip creating directories
}

func (d *Downloader) logf(format string, a ...any) {
	if d.Out != nil {
		fmt.Fprintf(d.Out, format+"\n", a...)
	}
}

func (d *Downloader) mkdir(dir string) error {
	if d.DryRun {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// DownloadURL downloads one ad-hoc URL straight into Root. If forced is Auto
// the tool is chosen from the URL.
func (d *Downloader) DownloadURL(ctx context.Context, rawURL string, forced Tool) error {
	tool := forced
	if tool == Auto {
		tool = ToolForURL(rawURL)
	}
	if err := d.mkdir(d.Root); err != nil {
		return err
	}
	d.logf("==> %s (%s)", rawURL, tool)
	name, args := Command(tool, rawURL)
	return d.Runner.Run(ctx, d.Root, name, args...)
}

// DownloadJob downloads one album into <Root>/<Artist>/<Album> and returns
// that directory.
func (d *Downloader) DownloadJob(ctx context.Context, j Job, forced Tool) (string, error) {
	tool, link, err := SelectSource(j.Entry, forced)
	if err != nil {
		return "", err
	}
	dir := AlbumDir(d.Root, j.Artist, j.Album)
	if err := d.mkdir(dir); err != nil {
		return "", err
	}
	name, args := Command(tool, link)
	return dir, d.Runner.Run(ctx, dir, name, args...)
}

// DownloadJobs runs every job in order. A failing job is recorded and the
// rest continue; cancelling ctx stops the loop early.
func (d *Downloader) DownloadJobs(ctx context.Context, jobs []Job, forced Tool) []Result {
	results := make([]Result, 0, len(jobs))
	for i, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		d.logf("==> [%d/%d] %s / %s", i+1, len(jobs), j.Artist, j.Album)
		dir, err := d.DownloadJob(ctx, j, forced)
		results = append(results, Result{Job: j, Dir: dir, Err: err})
	}
	return results
}

// Beet runs `beet import -C dir` (import in place, no copying) so beets can
// tag and organise what was just downloaded. beet is interactive, so the
// Runner should be wired to the terminal.
func Beet(ctx context.Context, r runner.Runner, dir string) error {
	return r.Run(ctx, "", "beet", "import", "-C", dir)
}
