// Package cli implements the mdl command line: option parsing, help text and
// every sub-command. All I/O goes through the App's fields so the whole thing
// can be driven from tests.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/projector22/ubiquitous-sniffle/internal/config"
	"github.com/projector22/ubiquitous-sniffle/internal/deps"
	"github.com/projector22/ubiquitous-sniffle/internal/downloader"
	"github.com/projector22/ubiquitous-sniffle/internal/library"
	"github.com/projector22/ubiquitous-sniffle/internal/runner"
	"github.com/projector22/ubiquitous-sniffle/internal/tui"
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
	Runner   runner.Runner                                            // runs external tools
	LookPath func(string) (string, error)                             // finds executables
	TUI      func(*library.Library, func() error) (tui.Result, error) // interactive browser; the func saves the library
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
	{'C', "config", "FILE", "Config file (settings such as the editor) to use. Default: $MDL_CONFIG, else ~/.config/mdl/config.json. Separate from the library."},
	{'m', "music-dir", "DIR", "Directory under which <Artist>/<Album> folders are created. Default: $MDL_MUSIC_DIR, else the current directory."},
	{'n', "dry-run", "", "Print the external commands that would run (downloaders, beet, installers, editor) without running or creating anything."},
}

// Options shared by the commands that download.
var downloadOpts = []Opt{
	{'s', "spotify", "", "Force Spotify (spotdl). Library albums use their Spotify link and fail if they have none; URLs are handed to spotdl whatever their host."},
	{'y', "youtube", "", "Force YouTube (yt-dlp). Library albums use their YouTube link and fail if they have none; URLs are handed to yt-dlp whatever their host. Cannot be combined with --spotify."},
	{'b', "beet", "", "After downloading, run `beet import -C` on each downloaded folder so beets can tag and organise it."},
	{'p', "audio", "LIST", "Where spotdl gets the audio for Spotify links, tried in order, e.g. \"bandcamp youtube-music\". Providers: youtube-music, youtube, bandcamp, soundcloud, piped. Overrides `mdl config audio`; spotdl's default is youtube-music. yt-dlp is not affected."},
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
	ctx        context.Context
	vals       *Values
	args       []string
	libPath    string
	configPath string
	musicDir   string
	dry        bool
	audio      []string // spotdl audio providers; nil for spotdl's default
}

func (a *App) commands() []command {
	return []command{
		{
			Name: "download", Aliases: []string{"dl", "get"}, Args: "[URL...]",
			Summary: "Download albums from the library, or the given URLs",
			Long: "With URLs, downloads each one straight into the current directory (or --music-dir): spotdl for Spotify " +
				"links, yt-dlp for YouTube links. This is also what plain `mdl URL` does, so `download` can be left out. " +
				"Add --to-library to file URL downloads into the music library instead, under the artist if the tracks " +
				"say who it is and under \"Unknown Artist\" if not. " +
				"With --file (or plain `mdl` in a directory that has a mdl.json) the albums of a batch file are downloaded " +
				"instead: a single-artist file into <dir>/<Album>/, where <dir> is the directory the file is in and is " +
				"taken to be the artist's folder, a multi-artist file into <dir>/<Artist>/<Album>/. --music-dir sets that " +
				"directory. Each URL gets the tool matching its host, so Spotify and YouTube links may be mixed. " +
				"Every URL that downloads successfully is also recorded in the library file (see --no-import), under " +
				"--artist/--album if given, else the folder --to-library filled, else the album named after the directory " +
				"you ran mdl in and the artist named after the directory above it. " +
				"With no URLs, downloads albums from the library into <music-dir>/<Artist>/<Album>/; use --artist and/or " +
				"--album to download a subset. Spotify links are preferred when an album has both; force a service with " +
				"--spotify or --youtube. A failed album does not stop the rest.",
			Opts: append(append([]Opt{}, downloadOpts...),
				Opt{'t', "to-library", "", "For URLs: file the download under <music-dir>/<Artist>/<Album>/ instead of the current directory. Artist and album come from the tracks' metadata; missing ones become \"Unknown Artist\" / \"Unknown Album\". With --beet only the new album folders are imported."},
				Opt{'f', "file", "FILE", "Download the albums listed in a batch file (see `mdl import`) instead of the library: {\"Album\": \"url\"} for one artist, whose folder is the directory the file is in, or {\"Artist\": {\"Album\": \"url\"}}. Each URL uses the right tool for its host unless --spotify or --youtube forces one. Plain `mdl` does this for ./mdl.json when it exists."},
				Opt{'d', "delete", "", "With --file: delete the batch file once every album has downloaded successfully."},
				Opt{'N', "no-import", "", "Do not record downloaded URLs in the library file. By default every URL you download is recorded there."},
				Opt{'a', "artist", "NAME", "Library downloads: only this artist's albums (case-insensitive). With URLs: the artist the download is recorded under (and, with --to-library, the artist folder)."},
				Opt{'A', "album", "NAME", "Library downloads: only albums with this name (case-insensitive). With URLs: the album the download is recorded under (and, with --to-library, the album folder)."}),
			Examples: []string{
				"mdl download",
				`mdl download -a "Daft Punk" -A Discovery`,
				"mdl download -yb https://music.youtube.com/playlist?list=...",
				"mdl https://open.spotify.com/album/...",
				"mdl                                      download ./mdl.json, if there is one",
				"mdl -m ~/Music/Abba                      download ~/Music/Abba/mdl.json into that folder",
				"mdl download --file albums.json -d       download a batch file, then delete it",
				"mdl -t https://music.youtube.com/playlist?list=...",
				`mdl -t -a "Daft Punk" -A Discovery https://youtu.be/...`,
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
				"press / to filter, d to download the selected album (or every album of the selected artist), " +
				"x to delete it from the library (after a y/n confirmation), and q to quit. " +
				"After you press d the TUI closes and the download runs.",
			Opts:     downloadOpts,
			Examples: []string{"mdl tui", "mdl tui -b"},
			run:      a.cmdTUI,
		},
		{
			Name:    "edit",
			Summary: "Open the library file in your editor",
			Long: "Opens the library in your editor, creating an empty library first if none exists, " +
				"and validates the file when the editor exits. The editor is --editor if given, else the one saved " +
				"with `mdl config editor`, else $VISUAL, else $EDITOR, else nano.",
			Opts:     []Opt{editorOpt},
			Examples: []string{"mdl edit", `mdl edit --editor "code --wait"`},
			run:      a.cmdEdit,
		},
		{
			Name: "config", Args: "[KEY [VALUE]]",
			Summary: "Show or change saved settings (such as the editor)",
			Long: "Settings live in a small JSON file, separate from the library (default ~/.config/mdl/config.json; " +
				"see --config). With no arguments, lists every setting, its value and where the value comes from. " +
				"With KEY, prints its value. With KEY and VALUE, saves it. With --unset, removes it. " +
				"Settings: editor - the command that `mdl edit` and `mdl -c` run, with arguments and quotes if needed. " +
				"The editor is chosen from: --editor, the config file, $VISUAL, $EDITOR, nano. " +
				"audio - where spotdl gets the audio for Spotify links, tried in order (youtube-music, youtube, bandcamp, " +
				"soundcloud, piped); chosen from --audio, the config file, then spotdl's default (youtube-music).",
			Opts: []Opt{
				{'u', "unset", "", "Remove KEY from the config file so the environment or the default applies again."},
			},
			Examples: []string{
				"mdl config                               list settings and where they come from",
				`mdl config editor "code --wait"          always use VS Code`,
				`mdl config editor "'/opt/My Editor/ed' -w"   quote paths that contain spaces`,
				"mdl config editor                        print the editor mdl will use",
				"mdl config --unset editor                go back to $VISUAL / $EDITOR / nano",
				`mdl config audio "bandcamp youtube-music"   try Bandcamp first, then YouTube Music`,
			},
			run: a.cmdConfig,
		},
		{
			Name: "import", Args: "[FILE]",
			Summary: "Import albums from a batch file, or create and edit one with -c",
			Long: "A batch file lists albums and their links in one of two shapes: {\"Album\": \"url\"} for a single artist, " +
				"or {\"Artist\": {\"Album\": \"url\"}} for several (the formats of the old shell and Python versions). " +
				"With --create, mdl first makes FILE (default ./mdl.json) from a sample, or reuses it if it already exists, " +
				"opens it in your editor, and imports it when the editor closes; nothing is downloaded. " +
				"Without --create, FILE must already exist. A single-artist file does not say whose albums they are: " +
				"they are filed under --artist if given, otherwise under the name of the directory the file is in. " +
				"A file with its own artists ignores --artist. Existing albums are updated, never removed. " +
				"With --create the editor is chosen as for `mdl edit` (see `mdl config`). " +
				"With no FILE, ./mdl.json is used (or <--music-dir>/mdl.json). --spotdll-log imports the master log " +
				"({\"Artist\": {\"Album\": \"url\"}}) that the old Python script kept in ~/.spotdll/log.json.",
			Opts: []Opt{
				{'c', "create", "", "Create FILE from a sample (or edit it if it exists) in your editor, then import it. Saving the sample untouched imports nothing."},
				{'a', "artist", "NAME", "Artist for a single-artist file. Default: the name of the directory containing FILE."},
				{'d', "delete", "", "Delete FILE after it has been imported successfully."},
				{'l', "spotdll-log", "", "Import the old spotdll script's master log (FILE, default ~/.spotdll/log.json) instead of a batch file. Unusable old entries are skipped and listed."},
				editorOpt,
			},
			Examples: []string{
				"mdl -c                                   create ./mdl.json, edit it, import it",
				`mdl import -c -a "Daft Punk"             same, filing a single-artist file under Daft Punk`,
				"mdl import -cd ~/Music/Abba/abba.json    edit and import that file, then delete it",
				"mdl import ~/Music/Daft\\ Punk/mdl.json    import an existing file (artist: Daft Punk, from the directory)",
				`mdl import old/mdl.json -a "Some Artist"`,
				"mdl import --spotdll-log                  import the old script's ~/.spotdll/log.json",
				"mdl -m ~/Music/Abba -c                   edit and import ~/Music/Abba/mdl.json",
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
	// `mdl [download options] URL...` is shorthand for `mdl download ...`.
	if dl := a.implicitDownload(args); dl != nil {
		return a.runCommand(ctx, dl, newValues(), args)
	}

	// `mdl -c [FILE]` is shorthand for `mdl import -c [FILE]`.
	if imp := a.implicitCreate(args); imp != nil {
		return a.runCommand(ctx, imp, newValues(), args)
	}

	// Plain `mdl` downloads ./mdl.json when there is one (the original
	// script's normal mode); `mdl -f FILE` names another batch file.
	if dl, g, code, handled := a.implicitBatch(args); handled {
		return code
	} else if dl != nil {
		return a.runCommand(ctx, dl, g, args)
	}

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
	return a.runCommand(ctx, cmd, globals, rest[1:])
}

// isURL reports whether s looks like a web link rather than a command name.
func isURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// implicitDownload returns the download command when args look like
// `[options] URL...`: the first argument that is not an option is a URL
// rather than a command name. Download options such as -s or --youtube are
// accepted before the URL.
func (a *App) implicitDownload(args []string) *command {
	dl := a.find("download")
	specs := append(append([]Opt{}, dl.Opts...), globalOpts...)
	_, rest, err := parse(specs, args, true)
	if err != nil || len(rest) == 0 || a.find(rest[0]) != nil || !isURL(rest[0]) {
		return nil
	}
	return dl
}

// implicitBatch handles invocations made only of download options: no
// command, no URL. With a batch file (named by --file, or <target
// directory>/mdl.json) it returns the download command and the options to run
// it with. Otherwise, if download-only options such as -s were given, the
// request cannot be met and handled is true with the exit code to return, so
// the user is told so instead of seeing "unknown option". Options shared with
// every command (-n, -L, ...) alone fall through to the plain help screen.
func (a *App) implicitBatch(args []string) (dl *command, g *Values, code int, handled bool) {
	dl = a.find("download")
	specs := append(append([]Opt{}, dl.Opts...), globalOpts...)
	vals, rest, err := parse(specs, args, true)
	if err != nil || len(rest) > 0 {
		return nil, nil, 0, false
	}
	downloadOnly := false
	for _, o := range dl.Opts {
		if vals.Has(o.Long) {
			downloadOnly = true
		}
	}
	switch {
	case vals.Bool("help"), vals.Bool("version"):
		if downloadOnly { // `mdl -s -h`: the help request wins over everything
			if vals.Bool("help") {
				fmt.Fprint(a.Stdout, a.topHelp())
			} else {
				fmt.Fprintf(a.Stdout, "mdl %s\n", a.Version)
			}
			return nil, nil, exitOK, true
		}
		return nil, nil, 0, false
	case vals.Has("file"):
		return dl, newValues(), 0, false
	}
	dir := vals.String("music-dir")
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, defaultBatchFile)
	if fileExists(path) {
		g = newValues()
		g.m["file"] = []string{path}
		return dl, g, 0, false
	}
	if downloadOnly {
		return nil, nil, a.usageErr(fmt.Errorf("nothing to download: there is no %s (give URLs, a command, or --file)", path)), true
	}
	return nil, nil, 0, false
}

// implicitCreate returns the import command when args are `[options] -c
// [FILE]`: --create given, and no command name or URL. It lets the old
// `script -c` habit work as plain `mdl -c`.
func (a *App) implicitCreate(args []string) *command {
	imp := a.find("import")
	specs := append(append([]Opt{}, imp.Opts...), globalOpts...)
	vals, rest, err := parse(specs, args, true)
	if err != nil || !vals.Bool("create") || len(rest) > 1 {
		return nil
	}
	if len(rest) == 1 && (a.find(rest[0]) != nil || isURL(rest[0])) {
		return nil
	}
	return imp
}

// runCommand parses cmd's own options from args, merges them with the global
// options already parsed, and runs it.
func (a *App) runCommand(ctx context.Context, cmd *command, globals *Values, args []string) int {
	specs := append(append([]Opt{}, cmd.Opts...), globalOpts...)
	vals, pos, err := parse(specs, args, false)
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
	inv.configPath = all.String("config")
	if inv.configPath == "" {
		inv.configPath = config.DefaultPath(a.Getenv, a.Home)
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
	switch s, y := v.Bool("spotify"), v.Bool("youtube"); {
	case s && y:
		return downloader.Auto, usagef("--spotify and --youtube cannot be used together")
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
	if inv.audio, err = a.audioProviders(inv); err != nil {
		return a.fail("%v", err)
	}

	toLibrary := inv.vals.Bool("to-library")
	file := inv.vals.String("file")
	switch {
	case file != "":
		switch {
		case len(inv.args) > 0:
			return a.usageErr(errors.New("--file cannot be combined with URLs"))
		case toLibrary:
			return a.usageErr(errors.New("--to-library only applies to URLs, not to --file"))
		case inv.vals.Has("album"):
			return a.usageErr(errors.New("--album does not apply to --file; the batch file names its albums"))
		}
		return a.downloadBatch(inv, file, forced)
	case inv.vals.Bool("delete"):
		return a.usageErr(errors.New("--delete only applies with --file"))
	}
	if len(inv.args) > 0 {
		return a.downloadURLs(inv, forced, toLibrary)
	}
	if toLibrary {
		return a.usageErr(errors.New("--to-library only applies to URLs; library downloads always use <Artist>/<Album>"))
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

// downloadURLs handles `mdl [-t] URL...`: download each URL, then (unless
// --no-import) record it in the library file.
func (a *App) downloadURLs(inv *invocation, forced downloader.Tool, toLibrary bool) int {
	// Plain URLs land in the current directory unless -m says otherwise
	// ($MDL_MUSIC_DIR only applies to the Artist/Album layout).
	root := inv.vals.String("music-dir")
	if root == "" {
		root = "."
	}
	if toLibrary {
		root = inv.musicDir
	}
	// The directory whose names stand in for a missing artist/album: where
	// the files land (the -m target, else the current directory). With
	// --to-library the library root says nothing about one album, so the
	// directory mdl was run in is used instead.
	fallbackDir := root
	if toLibrary {
		fallbackDir = "."
	}
	dl := a.newDownloader(inv, root)
	failed, seen := false, map[string]bool{}
	var dirs []string
	for _, u := range inv.args {
		var landed []string // album folders this URL produced (--to-library only)
		var err error
		if toLibrary {
			landed, err = dl.DownloadURLToLibrary(inv.ctx, u, forced, inv.vals.String("artist"), inv.vals.String("album"))
			for _, d := range landed {
				if err == nil && !seen[d] {
					seen[d] = true
					dirs = append(dirs, d)
				}
			}
		} else {
			err = dl.DownloadURL(inv.ctx, u, forced)
			if err == nil && !seen[root] {
				seen[root] = true
				dirs = append(dirs, root)
			}
		}
		if err != nil {
			fmt.Fprintf(a.Stderr, "mdl: %s: %v\n", u, err)
			failed = true
			continue
		}
		if !inv.vals.Bool("no-import") {
			if _, cerr := library.ClassifyURL(u); cerr != nil {
				// Downloaded fine, but only Spotify / YouTube links can be stored.
				fmt.Fprintf(a.Stderr, "mdl: not recorded in the library: %v\n", cerr)
			} else if err := a.recordURL(inv, u, landed, toLibrary, fallbackDir); err != nil {
				fmt.Fprintf(a.Stderr, "mdl: not recorded in the library: %v\n", err)
				failed = true
			}
		}
	}
	return a.finish(inv, dirs, failed)
}

// recordNames decides which artist and album a downloaded URL is filed under
// in the library file, in this order:
//
//  1. --artist / --album;
//  2. with --to-library, the folder the downloader actually filled, when it
//     produced exactly one (<Artist>/<Album>);
//  3. the fallback directory (where the files landed: --music-dir, else the
//     directory mdl was run in): its name is the album and the name of the
//     directory above it is the artist.
//
// Each of artist and album is resolved independently, so the library gets an
// entry no matter what the metadata lookup found.
func recordNames(flagArtist, flagAlbum string, landed []string, fallbackDir string) (artist, album string) {
	artist, album = flagArtist, flagAlbum
	if len(landed) == 1 {
		if artist == "" {
			artist = filepath.Base(filepath.Dir(landed[0]))
		}
		if album == "" {
			album = filepath.Base(landed[0])
		}
	}
	if artist == "" || album == "" {
		dirArtist, dirAlbum := downloader.NamesFromDir(fallbackDir)
		if artist == "" {
			artist = dirArtist
		}
		if album == "" {
			album = dirAlbum
		}
	}
	return artist, album
}

// recordURL stores rawURL (a Spotify or YouTube link) in the library file
// under the names chosen by recordNames.
func (a *App) recordURL(inv *invocation, rawURL string, landed []string, toLibrary bool, fallbackDir string) error {
	var entry library.Entry
	if err := entry.SetURL(rawURL); err != nil {
		return err
	}
	if !toLibrary {
		landed = nil
	}
	artist, album := recordNames(inv.vals.String("artist"), inv.vals.String("album"), landed, fallbackDir)
	return a.storeEntry(inv, artist, album, entry)
}

// storeEntry adds entry to the library file under artist / album (or, in a
// dry run, says that it would).
func (a *App) storeEntry(inv *invocation, artist, album string, entry library.Entry) error {
	lib, err := library.LoadOrNew(inv.libPath)
	if err != nil {
		return err
	}
	if canon, ok := lib.ResolveArtist(artist); ok {
		if al, ok := lib.ResolveAlbum(canon, album); ok {
			old, _ := lib.Get(canon, al)
			if old.Spotify != "" && entry.Spotify != "" && old.Spotify != entry.Spotify ||
				old.YouTube != "" && entry.YouTube != "" && old.YouTube != entry.YouTube {
				fmt.Fprintf(a.Stderr, "mdl: replacing the existing link for %s / %s in the library\n", canon, al)
			}
		}
	}
	if inv.dry {
		fmt.Fprintf(a.Stdout, "would record %s / %s in %s\n", artist, album, inv.libPath)
		return nil
	}
	if err := lib.Add(artist, album, entry); err != nil {
		return err
	}
	if err := lib.Save(inv.libPath); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "Recorded %s / %s in %s\n", artist, album, inv.libPath)
	return nil
}

// downloadBatch downloads the albums of a batch file. The directory holding
// the file is the target: a single-artist file's albums go straight into it
// (it is that artist's folder), a multi-artist file's into <dir>/<Artist>/
// <Album>. Every album that downloads is recorded in the library.
func (a *App) downloadBatch(inv *invocation, path string, forced downloader.Tool) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return a.fail("%v", err)
	}
	entries, err := library.ParseBatch(data)
	if err != nil {
		return a.fail("%s: %v", path, err)
	}
	dir := filepath.Dir(path)
	artist := a.defaultArtist(inv, dir)

	items := make([]downloader.Item, 0, len(entries))
	usedDefault := false
	for _, e := range entries {
		it := downloader.Item{Artist: e.Artist, Album: e.Album, URL: e.Entry.Spotify}
		if it.URL == "" {
			it.URL = e.Entry.YouTube
		}
		if e.Artist == "" {
			usedDefault = true
			it.Artist = artist
			it.Dir = filepath.Join(dir, downloader.SanitizeName(e.Album))
		} else {
			it.Dir = downloader.AlbumDir(dir, e.Artist, e.Album)
		}
		items = append(items, it)
	}
	a.noteArtist(inv, path, artist, usedDefault)
	if len(items) == 0 {
		fmt.Fprintf(a.Stdout, "Nothing to download in %s.\n", path)
		return exitOK
	}

	results := a.newDownloader(inv, dir).DownloadItems(inv.ctx, items, forced)
	failed := len(results) < len(items) // stopped early by cancellation
	var dirs []string
	for _, r := range results {
		if r.Err != nil {
			failed = true
			fmt.Fprintf(a.Stderr, "mdl: %s / %s: %v\n", r.Item.Artist, r.Item.Album, r.Err)
			continue
		}
		dirs = append(dirs, r.Item.Dir)
		if inv.vals.Bool("no-import") {
			continue
		}
		var entry library.Entry
		_ = entry.SetURL(r.Item.URL) // already classified by ParseBatch
		if err := a.storeEntry(inv, r.Item.Artist, r.Item.Album, entry); err != nil {
			fmt.Fprintf(a.Stderr, "mdl: not recorded in the library: %v\n", err)
			failed = true
		}
	}
	if !failed && inv.vals.Bool("delete") && !inv.dry {
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(a.Stderr, "mdl: deleting %s: %v\n", path, err)
			failed = true
		} else {
			fmt.Fprintf(a.Stdout, "Deleted %s\n", path)
		}
	}
	return a.finish(inv, dirs, failed)
}

// defaultArtist is the artist for the albums of a single-artist batch file in
// dir: --artist, else the name of dir (the artist's folder), else "Unknown
// Artist".
func (a *App) defaultArtist(inv *invocation, dir string) string {
	if v := inv.vals.String("artist"); v != "" {
		return v
	}
	if name, ok := downloader.NameOfDir(dir); ok {
		return name
	}
	return downloader.UnknownArtist
}

// noteArtist tells the user how the artist of a single-artist file was
// decided, or that --artist had no effect on a file that names its artists.
func (a *App) noteArtist(inv *invocation, path, artist string, usedDefault bool) {
	switch {
	case usedDefault && !inv.vals.Has("artist"):
		fmt.Fprintf(a.Stdout, "No --artist given: using %q, the name of the directory containing %s, as the artist.\n", artist, path)
	case !usedDefault && inv.vals.Has("artist"):
		fmt.Fprintf(a.Stderr, "mdl: --artist ignored: %s names its own artists\n", path)
	}
}

func (a *App) newDownloader(inv *invocation, root string) *downloader.Downloader {
	return &downloader.Downloader{Runner: a.runnerFor(inv), Root: root, Out: a.Stdout, DryRun: inv.dry, Audio: inv.audio}
}

func (a *App) runJobs(inv *invocation, jobs []downloader.Job, forced downloader.Tool) int {
	results := a.newDownloader(inv, inv.musicDir).DownloadJobs(inv.ctx, jobs, forced)
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
	if inv.audio, err = a.audioProviders(inv); err != nil {
		return a.fail("%v", err)
	}
	lib, code := a.loadLibrary(inv)
	if code != exitOK {
		return code
	}
	if lib.NumAlbums() == 0 {
		return a.fail("the library is empty; add albums with `mdl add` first")
	}
	save := func() error {
		if inv.dry {
			return nil
		}
		return lib.Save(inv.libPath)
	}
	res, err := a.TUI(lib, save)
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
	parts, err := a.editorCommand(inv)
	if err != nil {
		return a.fail("%v", err)
	}

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
	create, oldLog := inv.vals.Bool("create"), inv.vals.Bool("spotdll-log")
	if create && oldLog {
		return a.usageErr(errors.New("import: --create and --spotdll-log cannot be combined"))
	}
	var path string
	switch {
	case len(inv.args) > 1:
		return a.usageErr(errors.New("import: expected at most one FILE"))
	case len(inv.args) == 1:
		path = inv.args[0]
	case oldLog:
		path = filepath.Join(a.Home, ".spotdll", "log.json")
	case create:
		path = batchPath(inv)
	default:
		// Plain `mdl import` takes the batch file of the target directory.
		path = batchPath(inv)
		if !fileExists(path) {
			return a.usageErr(fmt.Errorf("import: expected a FILE (there is no %s; use --create to make one, or --spotdll-log for the old spotdll log)", path))
		}
	}
	if create {
		// A --music-dir that does not exist yet is created, so `mdl -m NEW -c` just works.
		if len(inv.args) == 0 && inv.vals.Has("music-dir") && !inv.dry {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return a.fail("%v", err)
			}
		}
		if code, proceed := a.editBatchFile(inv, path); !proceed {
			return code
		}
	}
	if oldLog {
		return a.importSpotdllLog(inv, path)
	}
	return a.importBatchFile(inv, path)
}

// defaultBatchFile is the batch file `mdl -c` creates and plain `mdl`
// downloads, like the spotdll.json of the original Python version.
const defaultBatchFile = "mdl.json"

// targetDir is the directory batch files are looked for in and downloaded
// into: --music-dir when given, else the current directory. ($MDL_MUSIC_DIR
// is deliberately not consulted: it is the root of the Artist/Album layout,
// not of one artist's batch file.)
func targetDir(inv *invocation) string {
	if d := inv.vals.String("music-dir"); d != "" {
		return d
	}
	return "."
}

// batchPath is the default batch file: <target directory>/mdl.json.
func batchPath(inv *invocation) string {
	return filepath.Join(targetDir(inv), defaultBatchFile)
}

// editBatchFile implements --create: make the file from the sample when it
// does not exist, open it in the editor, and report whether the caller should
// go on to import it. A freshly created file that was saved untouched is
// removed again, since the sample is only a placeholder.
func (a *App) editBatchFile(inv *invocation, path string) (code int, proceed bool) {
	template := library.BatchTemplate()
	_, statErr := os.Stat(path)
	existed := statErr == nil
	if !existed && !inv.dry {
		if err := os.WriteFile(path, template, 0o644); err != nil {
			return a.fail("creating %s: %v", path, err), false
		}
		fmt.Fprintf(a.Stderr, "Created %s - add albums as \"Album\": \"url\" (or \"Artist\": {\"Album\": \"url\"}).\n", path)
	}
	parts, err := a.editorCommand(inv)
	if err != nil {
		return a.fail("%v", err), false
	}
	if err := a.runnerFor(inv).Run(inv.ctx, "", parts[0], append(parts[1:], path)...); err != nil {
		return a.fail("%v", err), false
	}
	if inv.dry {
		return exitOK, false
	}
	if !existed {
		if after, err := os.ReadFile(path); err == nil && bytes.Equal(after, template) {
			_ = os.Remove(path)
			fmt.Fprintln(a.Stdout, "No changes made; nothing imported.")
			return exitOK, false
		}
	}
	return exitOK, true
}

// importBatchFile merges the batch file at path into the library.
func (a *App) importBatchFile(inv *invocation, path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return a.fail("%v", err)
	}
	artist := a.defaultArtist(inv, filepath.Dir(path))
	imported, usedDefault, err := library.ImportBatch(data, artist)
	if err != nil {
		return a.fail("%s: %v", path, err)
	}
	a.noteArtist(inv, path, artist, usedDefault)
	return a.mergeIntoLibrary(inv, imported, path, "")
}

// importSpotdllLog imports the old Python script's master log
// ({"Artist": {"Album": "url"}}, by default ~/.spotdll/log.json). Entries that
// can no longer be used are skipped and listed rather than failing the lot.
func (a *App) importSpotdllLog(inv *invocation, path string) int {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return a.fail("no spotdll log at %s (the old script kept it at ~/.spotdll/log.json; pass the file's path if it moved)", path)
	}
	if err != nil {
		return a.fail("%v", err)
	}
	entries, skipped, err := library.ParseLog(data)
	if err != nil {
		return a.fail("%s: %v", path, err)
	}
	imported := library.New()
	for _, e := range entries {
		if err := imported.Add(e.Artist, e.Album, e.Entry); err != nil {
			skipped = append(skipped, library.Skipped{Where: e.Artist + " / " + e.Album, Reason: err.Error()})
		}
	}
	for _, s := range skipped {
		fmt.Fprintf(a.Stderr, "mdl: skipped %s: %s\n", s.Where, s.Reason)
	}
	extra := ""
	if len(skipped) > 0 {
		extra = fmt.Sprintf(" (%d skipped)", len(skipped))
	}
	return a.mergeIntoLibrary(inv, imported, path, extra)
}

// mergeIntoLibrary adds imported to the library file, honouring --dry-run and
// --delete, and reports what happened. extra is appended to the summary.
func (a *App) mergeIntoLibrary(inv *invocation, imported *library.Library, path, extra string) int {
	lib, err := library.LoadOrNew(inv.libPath)
	if err != nil {
		return a.fail("%v", err)
	}
	n, err := lib.Merge(imported)
	if err != nil {
		return a.fail("%v", err)
	}
	switch {
	case inv.dry:
		fmt.Fprintf(a.Stdout, "would import %d album(s) into %s%s\n", n, inv.libPath, extra)
		return exitOK
	case n == 0:
		fmt.Fprintf(a.Stdout, "Nothing to import in %s.%s\n", path, extra)
	default:
		if err := lib.Save(inv.libPath); err != nil {
			return a.fail("saving library: %v", err)
		}
		fmt.Fprintf(a.Stdout, "Imported %d album(s) into %s%s\n", n, inv.libPath, extra)
	}
	if inv.vals.Bool("delete") {
		if err := os.Remove(path); err != nil {
			return a.fail("deleting %s: %v", path, err)
		}
		fmt.Fprintf(a.Stdout, "Deleted %s\n", path)
	}
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
