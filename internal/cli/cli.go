// Package cli implements the mdl command line: option parsing, help text and
// every sub-command. All I/O goes through the App's fields so the whole thing
// can be driven from tests.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"mdl/internal/deps"
	"mdl/internal/downloader"
	"mdl/internal/library"
	"mdl/internal/runner"
	"mdl/internal/tui"
)

// Process exit codes.
const (
	exitOK          = 0
	exitFailure     = 1
	exitUsage       = 2
	exitInterrupted = 130
)

// App wires the CLI to its environment.
type App struct {
	Version  string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Getenv   func(string) string
	Home     string
	Runner   runner.Runner                              // runs external tools
	LookPath func(string) (string, error)               // finds executables
	TUI      func(*library.Library) (tui.Result, error) // interactive browser
}

// NewApp returns an App connected to the real process environment.
func NewApp(version string) *App {
	home, _ := os.UserHomeDir()
	return &App{
		Version:  version,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Getenv:   os.Getenv,
		Home:     home,
		Runner:   runner.Exec{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr},
		LookPath: exec.LookPath,
		TUI:      tui.Run,
	}
}

// Options accepted by every command.
var globalOpts = []Opt{
	{'h', "help", "", "Show this help. After a command name it shows help for that command."},
	{'V', "version", "", "Print the mdl version and exit."},
	{'L', "library", "FILE", "Library file to use. Default: $MDL_LIBRARY, else ~/.config/mdl/library.json."},
	{'m', "music-dir", "DIR", "Directory under which <Artist>/<Album> folders are created. Default: $MDL_MUSIC_DIR, else the current directory."},
	{'n', "dry-run", "", "Print the external commands that would run (downloaders, beet, installers, editor) without running or creating anything."},
}

// Options shared by the commands that download.
var downloadOpts = []Opt{
	{'s', "spotdl", "", "Force spotdl. Albums without a Spotify link are reported as failures."},
	{'y', "yt-dlp", "", "Force yt-dlp. Albums without a YouTube link are reported as failures. Cannot be combined with --spotdl."},
	{'b', "beet", "", "After downloading, run `beet import -C` on each downloaded folder so beets can tag and organise it."},
}

type command struct {
	Name     string
	Aliases  []string
	Args     string
	Summary  string
	Long     string
	Opts     []Opt
	Examples []string
	run      func(*invocation) int
}

func (c command) usageName() string {
	if c.Args == "" {
		return c.Name
	}
	return c.Name + " " + c.Args
}

// invocation carries everything a command needs.
type invocation struct {
	ctx      context.Context
	vals     *Values
	args     []string
	libPath  string
	musicDir string
	dry      bool
}

func (a *App) commands() []command {
	return []command{
		{
			Name: "download", Aliases: []string{"dl", "get"}, Args: "[URL...]",
			Summary: "Download albums from the library, or the given URLs",
			Long: "With no URLs, downloads albums from the library into <music-dir>/<Artist>/<Album>/. " +
				"Use --artist and/or --album to download a subset. Spotify links are preferred when an album has both; " +
				"force a tool with --spotdl or --yt-dlp. With URLs, each one is downloaded straight into the music directory " +
				"using spotdl for Spotify links and yt-dlp for YouTube links. A failed album does not stop the rest.",
			Opts: append(append([]Opt{}, downloadOpts...),
				Opt{'a', "artist", "NAME", "Only download this artist's albums (case-insensitive). Library downloads only."},
				Opt{'A', "album", "NAME", "Only download albums with this name (case-insensitive). Library downloads only."}),
			Examples: []string{
				"mdl download",
				`mdl download -a "Daft Punk" -A Discovery`,
				"mdl download -yb https://music.youtube.com/playlist?list=...",
			},
			run: a.cmdDownload,
		},
		{
			Name: "add", Args: "-a ARTIST -A ALBUM -u URL...",
			Summary: "Add or update an album in the library",
			Long: "Stores a Spotify and/or YouTube link for an album. The service is detected from the URL. " +
				"Give --url twice to store both. Adding an existing album updates its links.",
			Opts: []Opt{
				{'a', "artist", "NAME", "Artist name (required)."},
				{'A', "album", "NAME", "Album name (required)."},
				{'u', "url", "URL", "Spotify or YouTube / YouTube Music link. May be repeated, once per service (required)."},
			},
			Examples: []string{`mdl add -a "Daft Punk" -A Discovery -u https://open.spotify.com/album/...`},
			run:      a.cmdAdd,
		},
		{
			Name: "list", Aliases: []string{"ls"},
			Summary: "Print the library as a tree",
			Long:    "Shows every artist and album with markers for which links are stored.",
			Opts: []Opt{
				{'a', "artist", "NAME", "Only show this artist."},
				{'u', "urls", "", "Also print each album's links."},
			},
			Examples: []string{"mdl list", `mdl list -u -a "Daft Punk"`},
			run:      a.cmdList,
		},
		{
			Name:    "tui",
			Summary: "Browse the library interactively",
			Long: "Opens a three-pane browser (artists, albums, details). Use the arrow keys or j/k/h/l, or click with the mouse; " +
				"press / to filter, d to download the selected album (or every album of the selected artist), and q to quit. " +
				"After you press d the TUI closes and the download runs.",
			Opts:     downloadOpts,
			Examples: []string{"mdl tui", "mdl tui -b"},
			run:      a.cmdTUI,
		},
		{
			Name:    "edit",
			Summary: "Open the library file in your editor",
			Long: "Opens the library in $VISUAL or $EDITOR (default vi), creating an empty library first if none exists, " +
				"and validates the file when the editor exits.",
			Opts: []Opt{{'e', "editor", "CMD", "Editor command to use instead of $VISUAL / $EDITOR. May include arguments, e.g. \"code -w\"."}},
			run:  a.cmdEdit,
		},
		{
			Name: "import", Args: "FILE",
			Summary: "Import a file written by the old shell script",
			Long: "Reads the previous formats - {\"Album\": \"url\"} for a single artist, or {\"Artist\": {\"Album\": \"url\"}} " +
				"for several - and merges them into the library.",
			Opts: []Opt{{'a', "artist", "NAME", "Artist for a single-artist file (required for that format)."}},
			Examples: []string{
				`mdl import old/mdl.json -a "Daft Punk"`,
				"mdl import ~/music/mdl.json",
			},
			run: a.cmdImport,
		},
		{
			Name: "beet", Args: "[DIR]",
			Summary: "Run beet on a folder",
			Long: "Runs `beet import -C DIR` (import in place, without copying) on DIR, or on the current directory if " +
				"omitted. beet is interactive and will prompt you.",
			Examples: []string{"mdl beet", "mdl beet ~/Music/Daft\\ Punk"},
			run:      a.cmdBeet,
		},
		{
			Name: "install", Args: "[beet|yt-dlp|spotdl...]",
			Summary: "Install beet, yt-dlp and spotdl",
			Long: "Installs the tools mdl drives, using pipx if available, otherwise `python -m pip install --user`. " +
				"With no names, installs all three. Tools that are already installed are skipped. " +
				"ffmpeg and node cannot be installed this way; mdl warns when they are missing.",
			Opts: []Opt{
				{'c', "check", "", "Only report which tools are installed; install nothing. Exits 1 if any is missing."},
				{'u', "upgrade", "", "Upgrade tools that are already installed."},
			},
			Examples: []string{"mdl install", "mdl install spotdl", "mdl install --check"},
			run:      a.cmdInstall,
		},
		{
			Name:    "version",
			Summary: "Print the mdl version",
			Long:    "Prints the version mdl was built with.",
			run:     func(*invocation) int { fmt.Fprintf(a.Stdout, "mdl %s\n", a.Version); return exitOK },
		},
		{
			Name: "help", Args: "[command]",
			Summary: "Show help for mdl or one command",
			Long:    "Same as --help; `mdl help download` equals `mdl download --help`.",
			run:     a.cmdHelp,
		},
	}
}

func (a *App) find(name string) *command {
	cmds := a.commands()
	for i := range cmds {
		if cmds[i].Name == name {
			return &cmds[i]
		}
		for _, al := range cmds[i].Aliases {
			if al == name {
				return &cmds[i]
			}
		}
	}
	return nil
}

func (a *App) usageErr(err error) int {
	fmt.Fprintf(a.Stderr, "mdl: %v\nTry 'mdl --help' for usage.\n", err)
	return exitUsage
}

func (a *App) fail(format string, args ...any) int {
	fmt.Fprintf(a.Stderr, "mdl: "+format+"\n", args...)
	return exitFailure
}

// Run executes mdl with args (excluding the program name) and returns the
// process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	globals, rest, err := parse(globalOpts, args, true)
	if err != nil {
		return a.usageErr(err)
	}
	if len(rest) == 0 {
		if globals.Bool("version") {
			fmt.Fprintf(a.Stdout, "mdl %s\n", a.Version)
			return exitOK
		}
		fmt.Fprint(a.Stdout, a.topHelp())
		return exitOK
	}

	cmd := a.find(rest[0])
	if cmd == nil {
		return a.usageErr(fmt.Errorf("unknown command %q", rest[0]))
	}
	specs := append(append([]Opt{}, cmd.Opts...), globalOpts...)
	vals, pos, err := parse(specs, rest[1:], false)
	if err != nil {
		return a.usageErr(fmt.Errorf("%s: %w", cmd.Name, err))
	}
	all := merge(globals, vals)

	switch {
	case all.Bool("help") && cmd.Name != "help":
		fmt.Fprint(a.Stdout, a.commandHelp(cmd))
		return exitOK
	case all.Bool("version"):
		fmt.Fprintf(a.Stdout, "mdl %s\n", a.Version)
		return exitOK
	}

	inv := &invocation{
		ctx:      ctx,
		vals:     all,
		args:     pos,
		libPath:  all.String("library"),
		musicDir: all.String("music-dir"),
		dry:      all.Bool("dry-run"),
	}
	if inv.libPath == "" {
		inv.libPath = library.DefaultPath(a.Getenv, a.Home)
	}
	if inv.musicDir == "" {
		inv.musicDir = a.Getenv("MDL_MUSIC_DIR")
	}
	if inv.musicDir == "" {
		inv.musicDir = "."
	}
	return cmd.run(inv)
}

func (a *App) runnerFor(inv *invocation) runner.Runner {
	if inv.dry {
		return runner.Dry{Out: a.Stdout}
	}
	return a.Runner
}

// loadLibrary loads an existing library, printing a helpful message if it is
// missing or invalid. The int is a non-zero exit code on failure.
func (a *App) loadLibrary(inv *invocation) (*library.Library, int) {
	lib, err := library.Load(inv.libPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, a.fail("no library at %s\nCreate one with `mdl add` or `mdl edit`, or migrate an old file with `mdl import`.", inv.libPath)
	}
	if err != nil {
		return nil, a.fail("%v", err)
	}
	return lib, exitOK
}

func forcedTool(v *Values) (downloader.Tool, error) {
	switch s, y := v.Bool("spotdl"), v.Bool("yt-dlp"); {
	case s && y:
		return downloader.Auto, usagef("--spotdl and --yt-dlp cannot be used together")
	case s:
		return downloader.Spotdl, nil
	case y:
		return downloader.YtDlp, nil
	}
	return downloader.Auto, nil
}

func (a *App) cmdHelp(inv *invocation) int {
	if len(inv.args) == 0 {
		fmt.Fprint(a.Stdout, a.topHelp())
		return exitOK
	}
	c := a.find(inv.args[0])
	if c == nil {
		return a.usageErr(fmt.Errorf("unknown command %q", inv.args[0]))
	}
	fmt.Fprint(a.Stdout, a.commandHelp(c))
	return exitOK
}

// --- download -------------------------------------------------------------

func (a *App) cmdDownload(inv *invocation) int {
	forced, err := forcedTool(inv.vals)
	if err != nil {
		return a.usageErr(err)
	}

	if len(inv.args) > 0 { // ad-hoc URLs
		if inv.vals.Has("artist") || inv.vals.Has("album") {
			return a.usageErr(errors.New("--artist and --album only apply to library downloads, not to URLs"))
		}
		dl := a.newDownloader(inv)
		failed, ok := false, false
		for _, u := range inv.args {
			if err := dl.DownloadURL(inv.ctx, u, forced); err != nil {
				fmt.Fprintf(a.Stderr, "mdl: %s: %v\n", u, err)
				failed = true
				continue
			}
			ok = true
		}
		var dirs []string
		if ok {
			dirs = []string{inv.musicDir}
		}
		return a.finish(inv, dirs, failed)
	}

	lib, code := a.loadLibrary(inv)
	if code != exitOK {
		return code
	}
	jobs, err := downloader.Plan(lib, inv.vals.String("artist"), inv.vals.String("album"))
	if err != nil {
		return a.fail("%v", err)
	}
	return a.runJobs(inv, jobs, forced)
}

func (a *App) newDownloader(inv *invocation) *downloader.Downloader {
	return &downloader.Downloader{Runner: a.runnerFor(inv), Root: inv.musicDir, Out: a.Stdout, DryRun: inv.dry}
}

func (a *App) runJobs(inv *invocation, jobs []downloader.Job, forced downloader.Tool) int {
	results := a.newDownloader(inv).DownloadJobs(inv.ctx, jobs, forced)
	var dirs []string
	failed := len(results) < len(jobs) // stopped early by cancellation
	for _, r := range results {
		if r.Err != nil {
			failed = true
			fmt.Fprintf(a.Stderr, "mdl: %s / %s: %v\n", r.Job.Artist, r.Job.Album, r.Err)
			continue
		}
		dirs = append(dirs, r.Dir)
	}
	return a.finish(inv, dirs, failed)
}

// finish runs beet on the downloaded directories (when requested) and turns
// the outcome into an exit code.
func (a *App) finish(inv *invocation, dirs []string, failed bool) int {
	if inv.ctx.Err() != nil {
		fmt.Fprintln(a.Stderr, "mdl: interrupted")
		return exitInterrupted
	}
	if inv.vals.Bool("beet") {
		for _, d := range dirs {
			fmt.Fprintf(a.Stdout, "==> beet import %s\n", d)
			if err := downloader.Beet(inv.ctx, a.runnerFor(inv), d); err != nil {
				fmt.Fprintf(a.Stderr, "mdl: %v\n", err)
				failed = true
			}
		}
	}
	if failed {
		return exitFailure
	}
	return exitOK
}

// --- library commands -----------------------------------------------------

func (a *App) cmdAdd(inv *invocation) int {
	v := inv.vals
	artist, album, urls := v.String("artist"), v.String("album"), v.Strings("url")
	switch {
	case artist == "" || album == "":
		return a.usageErr(errors.New("add: --artist and --album are required"))
	case len(urls) == 0:
		return a.usageErr(errors.New("add: at least one --url is required"))
	case len(inv.args) > 0:
		return a.usageErr(fmt.Errorf("add: unexpected argument %q (did you forget to quote a name?)", inv.args[0]))
	}
	var e library.Entry
	for _, u := range urls {
		if err := e.SetURL(u); err != nil {
			return a.fail("%v", err)
		}
	}
	lib, err := library.LoadOrNew(inv.libPath)
	if err != nil {
		return a.fail("%v", err)
	}
	if err := lib.Add(artist, album, e); err != nil {
		return a.fail("%v", err)
	}
	if inv.dry {
		fmt.Fprintf(a.Stdout, "would add %s / %s to %s\n", artist, album, inv.libPath)
		return exitOK
	}
	if err := lib.Save(inv.libPath); err != nil {
		return a.fail("saving library: %v", err)
	}
	fmt.Fprintf(a.Stdout, "Saved %s / %s to %s\n", artist, album, inv.libPath)
	return exitOK
}

func linkNames(e library.Entry) string {
	var n []string
	if e.Spotify != "" {
		n = append(n, "spotify")
	}
	if e.YouTube != "" {
		n = append(n, "youtube")
	}
	return strings.Join(n, ", ")
}

func (a *App) cmdList(inv *invocation) int {
	lib, code := a.loadLibrary(inv)
	if code != exitOK {
		return code
	}
	artists := lib.Artists()
	if want := inv.vals.String("artist"); want != "" {
		name, ok := lib.ResolveArtist(want)
		if !ok {
			return a.fail("artist %q not found in the library", want)
		}
		artists = []string{name}
	}
	showURLs := inv.vals.Bool("urls")
	for _, artist := range artists {
		albums := lib.Albums(artist)
		fmt.Fprintf(a.Stdout, "%s (%d)\n", artist, len(albums))
		for i, album := range albums {
			branch, pad := "├─", "│  "
			if i == len(albums)-1 {
				branch, pad = "└─", "   "
			}
			e, _ := lib.Get(artist, album)
			fmt.Fprintf(a.Stdout, "  %s %s  [%s]\n", branch, album, linkNames(e))
			if showURLs {
				if e.Spotify != "" {
					fmt.Fprintf(a.Stdout, "  %s    spotify %s\n", pad, e.Spotify)
				}
				if e.YouTube != "" {
					fmt.Fprintf(a.Stdout, "  %s    youtube %s\n", pad, e.YouTube)
				}
			}
		}
	}
	if len(artists) == 0 {
		fmt.Fprintf(a.Stdout, "The library is empty. Add albums with `mdl add`.\n")
	}
	return exitOK
}

func (a *App) cmdTUI(inv *invocation) int {
	forced, err := forcedTool(inv.vals)
	if err != nil {
		return a.usageErr(err)
	}
	lib, code := a.loadLibrary(inv)
	if code != exitOK {
		return code
	}
	if lib.NumAlbums() == 0 {
		return a.fail("the library is empty; add albums with `mdl add` first")
	}
	res, err := a.TUI(lib)
	if err != nil {
		return a.fail("%v", err)
	}
	if res.Action != tui.ActionDownload {
		return exitOK
	}
	jobs, err := downloader.Plan(lib, res.Artist, res.Album)
	if err != nil {
		return a.fail("%v", err)
	}
	return a.runJobs(inv, jobs, forced)
}

func (a *App) cmdEdit(inv *invocation) int {
	editor := inv.vals.String("editor")
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if editor == "" {
			editor = a.Getenv(name)
		}
	}
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)

	if _, err := os.Stat(inv.libPath); errors.Is(err, fs.ErrNotExist) && !inv.dry {
		if err := library.New().Save(inv.libPath); err != nil {
			return a.fail("creating library: %v", err)
		}
		fmt.Fprintf(a.Stderr, "Created %s\nFormat: {\"Artist\": {\"Album\": {\"spotify\": \"URL\", \"youtube\": \"URL\"}}}\n", inv.libPath)
	}
	if err := a.runnerFor(inv).Run(inv.ctx, "", parts[0], append(parts[1:], inv.libPath)...); err != nil {
		return a.fail("%v", err)
	}
	if inv.dry {
		return exitOK
	}
	if _, err := library.Load(inv.libPath); err != nil {
		return a.fail("the library is no longer valid: %v", err)
	}
	return exitOK
}

func (a *App) cmdImport(inv *invocation) int {
	if len(inv.args) != 1 {
		return a.usageErr(errors.New("import: expected exactly one FILE"))
	}
	data, err := os.ReadFile(inv.args[0])
	if err != nil {
		return a.fail("%v", err)
	}
	imported, err := library.ImportLegacy(data, inv.vals.String("artist"))
	if err != nil {
		return a.fail("%s: %v", inv.args[0], err)
	}
	lib, err := library.LoadOrNew(inv.libPath)
	if err != nil {
		return a.fail("%v", err)
	}
	n, err := lib.Merge(imported)
	if err != nil {
		return a.fail("%v", err)
	}
	if inv.dry {
		fmt.Fprintf(a.Stdout, "would import %d album(s) into %s\n", n, inv.libPath)
		return exitOK
	}
	if err := lib.Save(inv.libPath); err != nil {
		return a.fail("saving library: %v", err)
	}
	fmt.Fprintf(a.Stdout, "Imported %d album(s) into %s\n", n, inv.libPath)
	return exitOK
}

// --- tools ----------------------------------------------------------------

func (a *App) cmdBeet(inv *invocation) int {
	if len(inv.args) > 1 {
		return a.usageErr(errors.New("beet: expected at most one DIR"))
	}
	dir := "."
	if len(inv.args) == 1 {
		dir = inv.args[0]
	}
	if !inv.dry {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return a.fail("%s is not a directory", dir)
		}
	}
	if err := downloader.Beet(inv.ctx, a.runnerFor(inv), dir); err != nil {
		return a.fail("%v", err)
	}
	return exitOK
}

func (a *App) cmdInstall(inv *invocation) int {
	inst := &deps.Installer{Runner: a.runnerFor(inv), LookPath: a.LookPath, Out: a.Stdout, DryRun: inv.dry}
	if inv.vals.Bool("check") {
		missing := false
		for _, group := range [][]deps.Dep{deps.Managed, deps.Recommended} {
			for _, s := range inst.Check(group) {
				if s.Found {
					fmt.Fprintf(a.Stdout, "ok       %-8s %s\n", s.Binary, s.Path)
				} else {
					fmt.Fprintf(a.Stdout, "missing  %-8s %s\n", s.Binary, s.Purpose)
					missing = missing || s.Package != ""
				}
			}
		}
		if missing {
			return exitFailure
		}
		return exitOK
	}
	if err := inst.Install(inv.ctx, inv.args, inv.vals.Bool("upgrade")); err != nil {
		return a.fail("%v", err)
	}
	return exitOK
}
