package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/projector22/ubiquitous-sniffle/internal/library"
	"github.com/projector22/ubiquitous-sniffle/internal/runner"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func callNames(calls []runner.Call) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.Name)
	}
	return out
}

// batchJSON keeps keys deliberately out of alphabetical order.
const batchJSON = `{
  "Zebra Album": "` + spotURL + `",
  "Alpha Album": "` + ytURL + `",
  "Middle Album": "https://open.spotify.com/album/mid"
}`

func TestPlainMdlDownloadsMdlJsonInTheCurrentDirectory(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "Daft Punk")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)

	h.expect(t, 0) // plain `mdl`
	if len(h.fake.Calls) != 3 {
		t.Fatalf("calls = %+v", h.fake.Calls)
	}
	// File order is kept, each album goes into its own folder in the artist's folder,
	// and each URL gets the tool for its host (mixed Spotify / YouTube).
	want := []struct{ tool, dir string }{{"spotdl", "Zebra Album"}, {"yt-dlp", "Alpha Album"}, {"spotdl", "Middle Album"}}
	for i, w := range want {
		if c := h.fake.Calls[i]; c.Name != w.tool || c.Dir != w.dir {
			t.Errorf("call %d = %s in %q, want %s in %q", i, c.Name, c.Dir, w.tool, w.dir)
		}
		if !exists(filepath.Join(dir, w.dir)) {
			t.Errorf("folder %q was not created", w.dir)
		}
	}
	// Single-artist file: the artist is the directory it is in; every album is recorded.
	if !strings.Contains(h.out.String(), `using "Daft Punk"`) {
		t.Errorf("out = %q", h.out)
	}
	l := h.library(t)
	if e, ok := l.Get("Daft Punk", "Alpha Album"); !ok || e.YouTube != ytURL {
		t.Errorf("Alpha Album = %+v ok=%v", e, ok)
	}
	if e, _ := l.Get("Daft Punk", "Zebra Album"); e.Spotify != spotURL {
		t.Errorf("Zebra Album = %+v", e)
	}
	if !exists(filepath.Join(dir, "mdl.json")) {
		t.Error("the batch file is kept unless --delete is given")
	}
}

func TestPlainMdlWithoutBatchFileStillShowsHelp(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Empty")
	// Only options shared by every command: the plain help screen, as before.
	for _, args := range [][]string{{}, {"-n"}, {"-L", h.libPath}} {
		h.expect(t, 0, args...)
		if !strings.Contains(h.out.String(), "COMMANDS") {
			t.Errorf("mdl %v should print help without a mdl.json:\n%s", args, h.out)
		}
	}
	// Download options with nothing to download: a clear message, not "unknown option".
	for _, args := range [][]string{{"-s"}, {"--youtube"}, {"-b"}, {"-N"}, {"-a", "X"}} {
		h.expect(t, 2, args...)
		if !strings.Contains(h.err.String(), "nothing to download") || !strings.Contains(h.err.String(), "mdl.json") {
			t.Errorf("mdl %v: stderr = %q", args, h.err)
		}
	}
	if len(h.fake.Calls) != 0 {
		t.Errorf("nothing should run: %+v", h.fake.Calls)
	}
}

func TestHelpAndVersionBeatTheBatchFile(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)
	h.expect(t, 0, "-h")
	if !strings.Contains(h.out.String(), "COMMANDS") || len(h.fake.Calls) != 0 {
		t.Errorf("-h must show help, not download: %q %+v", h.out, h.fake.Calls)
	}
	h.expect(t, 0, "--version")
	if h.out.String() != "mdl test\n" || len(h.fake.Calls) != 0 {
		t.Errorf("--version: %q %+v", h.out, h.fake.Calls)
	}
	h.expect(t, 0, "-s", "-h")
	if !strings.Contains(h.out.String(), "COMMANDS") || len(h.fake.Calls) != 0 {
		t.Errorf("-h with other flags must still only show help: %q %+v", h.out, h.fake.Calls)
	}
	h.expect(t, 0, "-s", "-V")
	if h.out.String() != "mdl test\n" {
		t.Errorf("-V with other flags: %q", h.out)
	}
}

func TestBatchForcedToolIsRespected(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)

	for flag, want := range map[string]string{"-y": "yt-dlp", "--youtube": "yt-dlp", "-s": "spotdl", "--spotify": "spotdl", "--yt-dlp": "yt-dlp", "--spotdl": "spotdl"} {
		h.expect(t, 0, flag)
		for i, c := range h.fake.Calls {
			if c.Name != want {
				t.Errorf("mdl %s: call %d used %s, want %s everywhere", flag, i, c.Name, want)
			}
		}
	}
	// Without a force flag the tool follows each URL.
	h.expect(t, 0)
	if got := callNames(h.fake.Calls); !reflect.DeepEqual(got, []string{"spotdl", "yt-dlp", "spotdl"}) {
		t.Errorf("tools = %v", got)
	}
	h.expect(t, 2, "-s", "-y")
}

func TestBatchNestedFileUsesArtistFolders(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "Music")
	writeFile(t, filepath.Join(dir, "mdl.json"), `{
  "Daft Punk": {"Discovery": "`+spotURL+`"},
  "AC/DC": {"Back in Black": "`+ytURL+`"}
}`)
	h.expect(t, 0)
	if c := h.fake.Calls[0]; c.Dir != filepath.Join("Daft Punk", "Discovery") {
		t.Errorf("call 0 = %+v", c)
	}
	if c := h.fake.Calls[1]; c.Dir != filepath.Join("AC_DC", "Back in Black") {
		t.Errorf("call 1 = %+v", c)
	}
	l := h.library(t)
	if _, ok := l.Get("AC/DC", "Back in Black"); !ok { // recorded under the real name
		t.Errorf("artists = %v", l.Artists())
	}
	if strings.Contains(h.out.String(), "No --artist given") {
		t.Errorf("a nested file needs no fallback artist: %q", h.out)
	}
	// --artist cannot re-file a nested file.
	h.expect(t, 0, "-a", "Ignored")
	if !strings.Contains(h.err.String(), "--artist ignored") {
		t.Errorf("stderr = %q", h.err)
	}
	if _, ok := h.library(t).Get("Ignored", "Discovery"); ok {
		t.Error("--artist must not apply to a nested file")
	}
}

func TestBatchArtistFlagForSingleArtistFile(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "whatever")
	writeFile(t, filepath.Join(dir, "mdl.json"), `{"One": "`+spotURL+`"}`)
	h.expect(t, 0, "-a", "Given Artist")
	if _, ok := h.library(t).Get("Given Artist", "One"); !ok {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	// The folder is still <dir>/<Album>: the directory is the artist's folder.
	if h.fake.Calls[0].Dir != "One" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
}

func TestMusicDirIsTheTargetOfTheBatch(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Somewhere Else")
	target := filepath.Join(h.dir, "library", "Abba")
	writeFile(t, filepath.Join(target, "mdl.json"), `{"Arrival": "`+spotURL+`"}`)

	// -m names the directory holding mdl.json: albums go there, and the
	// artist is that directory's name, not the working directory's.
	h.expect(t, 0, "-m", target)
	if c := h.fake.Calls[0]; c.Dir != filepath.Join(target, "Arrival") {
		t.Errorf("call = %+v", c)
	}
	if _, ok := h.library(t).Get("Abba", "Arrival"); !ok {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	// $MDL_MUSIC_DIR is the Artist/Album root and is not used as a batch directory.
	h.withEnv(map[string]string{"MDL_MUSIC_DIR": target})
	h.expect(t, 0)
	if !strings.Contains(h.out.String(), "COMMANDS") {
		t.Errorf("$MDL_MUSIC_DIR must not select a batch file:\n%s", h.out)
	}
	// -m without a mdl.json there: help, as for plain mdl.
	h.expect(t, 0, "-m", filepath.Join(h.dir, "no-such-dir"))
	if !strings.Contains(h.out.String(), "COMMANDS") {
		t.Errorf("out = %q", h.out)
	}
}

func TestExplicitBatchFile(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "cwd")
	file := filepath.Join(h.dir, "elsewhere", "Some Artist", "albums.json")
	writeFile(t, file, `{"A": "`+spotURL+`"}`)

	for _, args := range [][]string{{"-f", file}, {"--file", file}, {"download", "--file", file}, {"download", "-f", file}} {
		h.expect(t, 0, args...)
		if c := h.fake.Calls[0]; c.Dir != filepath.Join(filepath.Dir(file), "A") {
			t.Errorf("mdl %v: call = %+v", args, c)
		}
		if _, ok := h.library(t).Get("Some Artist", "A"); !ok {
			t.Errorf("mdl %v: artists = %v", args, h.library(t).Artists())
		}
	}
	h.expect(t, 1, "-f", filepath.Join(h.dir, "missing.json"))
}

func TestBatchRecordingCanBeSkippedAndDeleteAfterSuccess(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	file := filepath.Join(dir, "mdl.json")
	writeFile(t, file, batchJSON)

	h.expect(t, 0, "-N")
	if exists(h.libPath) {
		t.Error("--no-import must not create the library")
	}
	h.expect(t, 0, "--delete")
	if exists(file) {
		t.Error("--delete should remove the batch file after success")
	}
	if !strings.Contains(h.out.String(), "Deleted ") {
		t.Errorf("out = %q", h.out)
	}

	// A failure keeps the file.
	writeFile(t, file, batchJSON)
	h.fake.Err = func(c runner.Call) error {
		if c.Dir == "Alpha Album" {
			return errors.New("boom")
		}
		return nil
	}
	h.expect(t, 1, "-d")
	if !exists(file) {
		t.Error("a batch with failures must not be deleted")
	}
	// Dry run never deletes.
	h.fake.Err = nil
	h.expect(t, 0, "-n", "-d")
	if !exists(file) {
		t.Error("dry run must not delete")
	}
}

func TestBatchFailuresContinueAndOnlySuccessesAreRecordedAndBeeted(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "Artist")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)
	h.fake.Err = func(c runner.Call) error {
		if c.Dir == "Alpha Album" {
			return errors.New("network down")
		}
		return nil
	}
	h.expect(t, 1, "-b")
	if !strings.Contains(h.err.String(), "Artist / Alpha Album: network down") {
		t.Errorf("stderr = %q", h.err)
	}
	l := h.library(t)
	if _, ok := l.Get("Artist", "Alpha Album"); ok {
		t.Error("a failed album must not be recorded")
	}
	if l.NumAlbums() != 2 {
		t.Errorf("albums = %d, want the 2 that worked", l.NumAlbums())
	}
	var beets []string
	for _, c := range h.fake.Calls {
		if c.Name == "beet" {
			beets = append(beets, c.Args[len(c.Args)-1])
		}
	}
	if !reflect.DeepEqual(beets, []string{"Zebra Album", "Middle Album"}) {
		t.Errorf("beet ran on %v", beets)
	}
}

func TestBatchDryRun(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "Artist")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)
	h.expect(t, 0, "-n")
	out := h.out.String()
	for _, want := range []string{"+ (cd 'Zebra Album' && spotdl", "+ (cd 'Alpha Album' && yt-dlp", "would record Artist / Zebra Album"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if exists(filepath.Join(dir, "Zebra Album")) || exists(h.libPath) {
		t.Error("dry run must not create folders or the library")
	}
}

func TestBatchProblems(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	file := filepath.Join(dir, "mdl.json")

	// Only the untouched sample: nothing to do.
	writeFile(t, file, string(library.BatchTemplate()))
	h.expect(t, 0)
	if !strings.Contains(h.out.String(), "Nothing to download") || len(h.fake.Calls) != 0 {
		t.Errorf("out = %q calls = %+v", h.out, h.fake.Calls)
	}
	// Invalid JSON, and a URL that is neither Spotify nor YouTube: nothing is downloaded.
	for _, content := range []string{`{bad`, `{"X": "https://example.com/x"}`, `[]`} {
		writeFile(t, file, content)
		h.expect(t, 1)
		if len(h.fake.Calls) != 0 || !strings.Contains(h.err.String(), "mdl.json") {
			t.Errorf("%s: calls=%+v stderr=%q", content, h.fake.Calls, h.err)
		}
	}
	// A library that cannot be read: the download works, the exit status says otherwise.
	writeFile(t, file, `{"X": "`+spotURL+`"}`)
	writeFile(t, h.libPath, "{broken")
	h.expect(t, 1)
	if len(h.fake.Calls) != 1 || !strings.Contains(h.err.String(), "not recorded in the library") {
		t.Errorf("calls=%+v stderr=%q", h.fake.Calls, h.err)
	}
}

func TestBatchUsageErrors(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	file := filepath.Join(dir, "mdl.json")
	writeFile(t, file, batchJSON)
	for _, args := range [][]string{
		{"download", "-f", file, spotURL}, // URLs and a file
		{"download", "-f", file, "-t"},    // --to-library with a file
		{"download", "-f", file, "-A", "x"},
		{"download", "-d"}, // --delete without --file
		{"-d", spotURL},
	} {
		if code := h.run(args...); code != 2 {
			t.Errorf("mdl %q: exit %d, want 2 (stderr %q)", args, code, h.err)
		}
	}
	if len(h.fake.Calls) != 0 {
		t.Errorf("nothing should have run: %+v", h.fake.Calls)
	}
}

func TestBatchInterrupted(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "A")
	writeFile(t, filepath.Join(dir, "mdl.json"), batchJSON)
	ctx, cancel := contextWithCancel()
	h.fake.Err = func(runner.Call) error { cancel(); return nil }
	if code := h.app.Run(ctx, nil); code != 130 {
		t.Errorf("exit = %d, want 130", code)
	}
	if file := filepath.Join(dir, "mdl.json"); !exists(file) {
		t.Error("an interrupted batch must keep its file")
	}
}

func TestPlainMdlTrailingDoubleDashDoesNotBreakDispatch(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Empty")
	h.expect(t, 0, "--")
}

// --- -m for plain URL downloads -------------------------------------------

func TestURLRecordNamesFollowTheMusicDirTarget(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Unrelated/Cwd")
	target := filepath.Join(h.dir, "Music", "Daft Punk", "Discovery")
	h.expect(t, 0, "-m", target, spotURL)
	if c := h.fake.Calls[0]; c.Dir != target {
		t.Errorf("call = %+v", c)
	}
	// Names come from where the files landed (-m), not from the working directory.
	if e, ok := h.library(t).Get("Daft Punk", "Discovery"); !ok || e.Spotify != spotURL {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	if _, ok := h.library(t).Get("Unrelated", "Cwd"); ok {
		t.Error("the working directory must not be used when -m names the target")
	}
	// Without -m nothing changes: the current directory.
	h.expect(t, 0, ytURL)
	if _, ok := h.library(t).Get("Unrelated", "Cwd"); !ok {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
}

// --- import: default file, -m, and the old spotdll log ---------------------

func TestBareImportUsesTheDefaultBatchFile(t *testing.T) {
	h := newHarness(t)
	dir := h.chdir(t, "Abba")
	writeFile(t, filepath.Join(dir, "mdl.json"), `{"Arrival": "`+spotURL+`"}`)
	h.expect(t, 0, "import")
	if _, ok := h.library(t).Get("Abba", "Arrival"); !ok {
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	h.chdir(t, "../Nothing")
	h.expect(t, 2, "import")
	if !strings.Contains(h.err.String(), "mdl.json") {
		t.Errorf("the error should name the file it looked for: %q", h.err)
	}
}

func TestImportMusicDirTargetsTheBatchFile(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "elsewhere")
	target := filepath.Join(h.dir, "Music", "Abba")

	// -c with a -m directory that does not exist yet creates it.
	h.edit(strp(`{"Arrival": "` + spotURL + `"}`))
	h.expect(t, 0, "-m", target, "-c")
	if c := h.fake.Calls[0]; c.Args[len(c.Args)-1] != filepath.Join(target, "mdl.json") {
		t.Errorf("editor call = %+v", c)
	}
	if !exists(filepath.Join(target, "mdl.json")) {
		t.Error("the batch file should live in the -m directory")
	}
	if _, ok := h.library(t).Get("Abba", "Arrival"); !ok { // artist = the -m directory's name
		t.Errorf("artists = %v", h.library(t).Artists())
	}
	// Plain import takes the same file.
	h.expect(t, 0, "-m", target, "import")
	// An explicit FILE is used as given, and its parent is not created.
	h.expect(t, 1, "import", "-c", filepath.Join(h.dir, "nope", "x.json"))
	// A dry run creates neither the directory nor the file.
	fresh := filepath.Join(h.dir, "fresh-dir")
	h.expect(t, 0, "-n", "-m", fresh, "-c")
	if exists(fresh) {
		t.Error("dry run must not create the -m directory")
	}
}

const spotdllLog = `{
  "Daft Punk": {
    "Discovery": "https://open.spotify.com/album/aaa",
    "Homework": "https://open.spotify.com/album/bbb"
  },
  "Abba": {"Arrival": "https://youtu.be/ccc"},
  "Broken Artist": {
    "Bad Host": "https://example.com/nope",
    "Not A String": {"spotify": "x"},
    "Fine": "https://open.spotify.com/album/ddd"
  },
  "Loose Album": "https://open.spotify.com/album/eee",
  "Empty Artist": null
}`

func TestImportSpotdllLog(t *testing.T) {
	h := newHarness(t)
	logPath := filepath.Join(h.dir, ".spotdll", "log.json") // ~/.spotdll/log.json, as the old script wrote it
	writeFile(t, logPath, spotdllLog)

	h.expect(t, 0, "import", "--spotdll-log")
	out := h.out.String()
	if !strings.Contains(out, "Imported 4 album(s) into "+h.libPath) || !strings.Contains(out, "(3 skipped)") {
		t.Errorf("out = %q", out)
	}
	// Every skipped entry is listed with its reason, and nothing aborted the import.
	for _, want := range []string{"skipped Broken Artist / Bad Host", "skipped Broken Artist / Not A String", "skipped Loose Album: no artist"} {
		if !strings.Contains(h.err.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, h.err)
		}
	}
	l := h.library(t)
	if e, _ := l.Get("Daft Punk", "Homework"); e.Spotify != "https://open.spotify.com/album/bbb" {
		t.Errorf("Homework = %+v", e)
	}
	if e, _ := l.Get("Abba", "Arrival"); e.YouTube != "https://youtu.be/ccc" {
		t.Errorf("Arrival = %+v", e)
	}
	if _, ok := l.Get("Broken Artist", "Fine"); !ok {
		t.Error("the usable entry of a partly broken artist should be imported")
	}
	if !exists(logPath) {
		t.Error("the old log must be left alone unless --delete is given")
	}

	// -l is the short form; importing again merges rather than duplicating.
	h.expect(t, 0, "import", "-l")
	if h.library(t).NumAlbums() != 4 {
		t.Errorf("albums = %d", h.library(t).NumAlbums())
	}
	// Explicit path, dry run, and --delete.
	copyPath := filepath.Join(h.dir, "copy.json")
	writeFile(t, copyPath, spotdllLog)
	h.expect(t, 0, "-n", "import", "-l", copyPath)
	if !strings.Contains(h.out.String(), "would import 4 album(s)") || !exists(copyPath) {
		t.Errorf("dry run: %q", h.out)
	}
	h.expect(t, 0, "import", "-ld", copyPath)
	if exists(copyPath) {
		t.Error("--delete should remove the log")
	}
}

func TestImportSpotdllLogProblems(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 1, "import", "--spotdll-log") // no ~/.spotdll/log.json
	if !strings.Contains(h.err.String(), ".spotdll/log.json") {
		t.Errorf("the error should say where the old log lives: %q", h.err)
	}
	h.expect(t, 2, "import", "-l", "-c")
	h.expect(t, 2, "import", "-l", "a", "b")
	bad := filepath.Join(h.dir, "bad.json")
	writeFile(t, bad, `{nope`)
	h.expect(t, 1, "import", "-l", bad)
	// A log with nothing usable imports nothing, without failing.
	empty := filepath.Join(h.dir, "empty.json")
	writeFile(t, empty, `{"A": {"B": "https://example.com"}}`)
	h.expect(t, 0, "import", "-l", empty)
	if !strings.Contains(h.out.String(), "Nothing to import") {
		t.Errorf("out = %q", h.out)
	}
	// A library that cannot be read.
	writeFile(t, h.libPath, "{broken")
	good := filepath.Join(h.dir, "good.json")
	writeFile(t, good, `{"A": {"B": "`+spotURL+`"}}`)
	h.expect(t, 1, "import", "-l", good)
}

func TestHelpMentionsBatchDownloadAndOldLog(t *testing.T) {
	a := &App{}
	top := a.topHelp()
	for _, want := range []string{"download ./mdl.json, if there is one"} {
		if !strings.Contains(top, want) {
			t.Errorf("top help missing %q", want)
		}
	}
	dl := strings.Join(strings.Fields(a.commandHelp(a.find("download"))), " ")
	for _, want := range []string{"--file", "--delete", "mdl.json", "Spotify and YouTube links may be mixed"} {
		if !strings.Contains(dl, want) {
			t.Errorf("download help missing %q", want)
		}
	}
	imp := strings.Join(strings.Fields(a.commandHelp(a.find("import"))), " ")
	for _, want := range []string{"--spotdll-log", "~/.spotdll/log.json", "--music-dir"} {
		if !strings.Contains(imp, want) {
			t.Errorf("import help missing %q", want)
		}
	}
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
