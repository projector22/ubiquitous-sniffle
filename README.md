# mdl

Download, organise and browse a personal music library from the terminal.

`mdl` keeps **one library file** mapping *artist → album → Spotify / YouTube link*,
downloads those albums with [spotdl](https://github.com/spotDL/spotify-downloader)
or [yt-dlp](https://github.com/yt-dlp/yt-dlp) into `<music-dir>/<Artist>/<Album>/`,
and can hand the result to [beets](https://beets.io) for tagging.

It is a single static binary. The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); everything else uses only the standard library.

## Install

```sh
make install              # builds and installs to ~/.local/bin/mdl (INSTALL_DIR=... to change)
export PATH="$HOME/.local/bin:$PATH"   # if ~/.local/bin is not already on your PATH
mdl install               # installs beet, yt-dlp and spotdl via pipx (or pip --user)
mdl install --check       # see what is present
```

You also need **ffmpeg** (and **node** for yt-dlp's `--js-runtimes node`); `mdl install` warns if they are
missing but leaves them to your system package manager.

Prebuilt archives for Linux, macOS and Windows are attached to each
[GitHub release](../../releases).

## Usage

The simplest use: give it a link and it downloads into the current directory.

```sh
mdl https://open.spotify.com/album/...            # spotdl, into the current directory
mdl https://music.youtube.com/playlist?list=...   # yt-dlp, into the current directory
mdl -b https://open.spotify.com/album/...         # ...then run beet on it
mdl --youtube URL                                 # force yt-dlp (or --spotify to force spotdl)
```

Every URL that downloads successfully is also **recorded in your library file** (see below), so the library
always reflects what you have fetched. `mdl URL...` is shorthand for `mdl download URL...`. The tool is picked from the link (Spotify → spotdl,
YouTube → yt-dlp); `-s`/`--spotify` and `-y`/`--youtube` override that. The older names `--spotdl` and
`--yt-dlp` still work. Use `-m DIR` to download somewhere else.

### One-off albums straight into the library

Add `-t` / `--to-library` to file a URL into the music library layout instead of the current directory:

```sh
mdl -t https://music.youtube.com/playlist?list=...     # <music-dir>/<Artist>/<Album>/01 - Track.mp3 ...
mdl -tb https://open.spotify.com/album/...             # ...and run beet on just the new album folder(s)
mdl -t -a "Daft Punk" -A Discovery https://youtu.be/...   # you already know who/what it is
```

The downloader names the folders from each track's own metadata, so a playlist or album lands under its
artist and album. When the tracks carry no artist, the files go under `Unknown Artist`, and with no album
under the playlist's title, or `Unknown Album` for a single video. `-a` / `-A` override either part. Track
numbers are zero-padded for playlists and albums. The music directory is `-m DIR`, else `$MDL_MUSIC_DIR`,
else the current directory.

Notes: with `--beet`, only album folders that were created or changed by this download are imported, never
your whole collection. Folder names that come from metadata are cleaned by yt-dlp / spotdl themselves, so a
`/` in a name can differ slightly from the `_` that `mdl download` uses for library entries (for example
`AC⧸DC` versus `AC_DC`); pass `-a` to get mdl's naming. spotdl always knows artist and album from Spotify, so
`Unknown Artist` only happens with YouTube sources.

### Every download is recorded

After each successful URL download, mdl adds the link to the library file, whatever the downloader's
metadata said. The entry is filed under:

1. `-a ARTIST` / `-A ALBUM`, if you gave them;
2. with `-t`, the `<Artist>/<Album>` folder the download actually filled (when it produced exactly one);
3. otherwise **the directory the files were downloaded into** (the one you ran mdl in, or the one given with
   `-m`): its name is the album and the name of the directory above it is the artist.

Each of artist and album is resolved separately, so `-A "Homework"` alone still takes the artist from the
directory above. For example, in `~/Music/Daft Punk/Discovery`:

```sh
cd ~/Music/Daft\ Punk/Discovery
mdl https://open.spotify.com/album/...        # recorded as Daft Punk / Discovery (spotify)
mdl https://music.youtube.com/playlist?...    # same album, adds the youtube link
mdl -N URL                                    # -N / --no-import: download only, record nothing
```

Details: only Spotify and YouTube links can be stored (anything else downloads with a warning); a URL that
fails to download is not recorded; running it somewhere unrelated, like `~/Downloads`, will file the album
under that folder's names, so pass `-a`/`-A` or `-N` there; if a different link for the same service is already
stored, it is replaced and mdl says so. Recorded entries can be fixed later with `mdl edit`.

### A managed library

```sh
mdl add -a "Daft Punk" -A Discovery -u https://open.spotify.com/album/...
mdl add -a "Daft Punk" -A Discovery -u https://music.youtube.com/playlist?list=...   # adds the YouTube link too
mdl list -u                       # tree of everything (-u also prints links)
mdl tui                           # browse; press d to download the selection
mdl download                      # every album in the library
mdl download -a "Daft Punk" -sb   # one artist, force Spotify links, then run beet on each folder
mdl download --youtube            # use each album's YouTube link instead
mdl beet ~/Music/Daft\ Punk       # run beet on a folder (default: current directory)
mdl edit                          # open the library in your editor (see Configuration)
mdl -n download                   # dry run: show the commands, change nothing
```

Run `mdl --help` for the full reference, or `mdl <command> --help`.

Every option has a short and a long form (`-s`/`--spotdl`, `-b`/`--beet`, `-a`/`--artist`, …),
short flags combine (`-sb`), and values can be attached (`--artist=NAME`, `-aNAME`).

### Batch files: `mdl -c`

For adding several albums at once, `-c` (the old `spotdll.py -c`) makes a small JSON file, opens it in your
editor, and imports it into the library when you close the editor. Creating and importing downloads nothing;
see [Downloading a batch file](#downloading-a-batch-file) for that.

```sh
cd ~/Music/Daft\ Punk
mdl -c                    # creates ./mdl.json from a sample (or edits it if it exists), then imports it
mdl -cd                   # the same, and delete mdl.json after a successful import
mdl -c -a "Daft Punk"     # name the artist explicitly
mdl import -c path/to/albums.json    # use another file (mdl -c path/to/albums.json also works)
```

The file can have either shape, and mdl works out which:

```json
{ "Discovery": "https://open.spotify.com/album/...", "Homework": "https://music.youtube.com/playlist?list=..." }
```

```json
{ "Daft Punk": { "Discovery": "https://open.spotify.com/album/..." }, "Abba": { "Arrival": "https://youtu.be/..." } }
```

- **Artist defined in the file** (second shape): the artists come from the file; `--artist` is ignored (with a note).
- **Artist defined with `-a`** (first shape): all albums go under that artist.
- **No artist at all** (first shape, no `-a`): the artist is the name of the directory the file is in, just as
  the Python version treated the current directory as the artist folder. mdl says which name it used.

Details: the sample is a single placeholder album, and mdl ignores that placeholder, so saving the sample
untouched imports nothing (a freshly created file left untouched is removed again). Links are classified as
Spotify or YouTube automatically; anything else is an error and nothing from that file is imported. Existing
albums are updated, never removed. `mdl import FILE` does the import without opening an editor, and plain
`mdl import` takes `./mdl.json`.

#### Downloading a batch file

Run plain `mdl` in a directory that has a `mdl.json` and it downloads the albums in it, just as `spotdll.py` did:

```sh
cd ~/Music/Daft\ Punk
mdl                       # download ./mdl.json
mdl -b                    # ...and run beet on each album folder afterwards
mdl -m ~/Music/Abba       # use ~/Music/Abba/mdl.json instead, and download into ~/Music/Abba
mdl -f albums.json -d     # a batch file anywhere (--file); delete it once everything succeeded (--delete)
mdl                       # without a mdl.json, you just get the help screen
```

- **Where albums go:** a single-artist file (`{"Album": "url"}`) is for one artist's folder, so the directory
  holding the file *is* the artist's folder and each album goes into `<dir>/<Album>/`. A file with artists
  (`{"Artist": {"Album": "url"}}`) uses `<dir>/<Artist>/<Album>/` below that directory. Albums are downloaded
  in the order they appear in the file.
- **Which tool:** each URL gets the tool for its own link (Spotify → spotdl, YouTube → yt-dlp), so a file may mix
  both freely. `-s` / `--spotify` or `-y` / `--youtube` forces one tool for every URL instead.
- **Recorded too:** like every download, each album that succeeds is recorded in the library (artist from the file,
  `-a`, or the directory's name; `-N` turns that off). A failed album does not stop the others and is not recorded.
- **`-m DIR`** is the target directory: it is where `mdl.json` is read from and downloaded into, and where
  `mdl import` / `mdl -c` look for and create it (a `-m` directory that does not exist yet is created by `-c`).
  `$MDL_MUSIC_DIR` is not used for this; it is only the root of the `Artist/Album` layout.

### Folder layout

```
<music-dir>/
  Daft Punk/
    Discovery/
    Homework/
  AC_DC/                 # "/" and other characters illegal in file names become "_"
    Back in Black/
```

Each downloader is started *inside* the album folder, so albums can never nest inside one another.

### The library file

One JSON file, sorted and indented so it is easy to read, diff and edit by hand:

```json
{
  "Daft Punk": {
    "Discovery": {
      "spotify": "https://open.spotify.com/album/2noRn2Aes5aoNVsU6iWThc",
      "youtube": "https://music.youtube.com/playlist?list=OLAK5uy_..."
    }
  }
}
```

Each album needs at least one link. When it has both, Spotify is used unless you pass `--youtube`; with `--spotify` or `--youtube`, albums lacking that link are reported as failures and the rest continue.
Unknown keys are rejected, so a typo like `"youtbe"` is reported instead of ignored.

Location: `-L FILE`, else `$MDL_LIBRARY`, else `~/.config/mdl/library.json`.
Music directory for the `Artist/Album` layout: `-m DIR`, else `$MDL_MUSIC_DIR`, else the current directory. Plain URL downloads and batch files ignore `$MDL_MUSIC_DIR` and use the current directory unless `-m` is given; then `-m` is also the directory whose name decides the artist and album a plain URL is recorded under.

### Configuration

Settings are saved in a small JSON file, **separate from the library**: `-C FILE`, else `$MDL_CONFIG`, else
`~/.config/mdl/config.json`. Manage it with `mdl config` instead of editing it by hand:

```sh
mdl config                              # list settings and where each value comes from
mdl config editor "code --wait"         # always use this editor (arguments allowed)
mdl config editor "'/opt/My Editor/ed' -w"   # quote paths that contain spaces
mdl config editor                       # print the editor mdl will use
mdl config --unset editor               # forget it again
```

The **editor** is used by `mdl edit` and `mdl -c`. It is chosen from, first match wins:

1. `--editor CMD` (`-e`) for a single run
2. the `editor` setting in the config file
3. `$VISUAL`
4. `$EDITOR`
5. `nano`

As with git's `core.editor`, a saved choice beats the environment variables. mdl warns, but still saves, if the
command is not on your `PATH`. A config file that cannot be read (bad JSON, an unknown key) is reported and never
overwritten.

### TUI

`mdl tui` shows artists, albums and details side by side.

| Key | Action |
| --- | --- |
| `↑` `↓` / `j` `k` | move |
| `←` `→` / `h` `l` / `Tab` | switch pane |
| click, mouse wheel | select / scroll |
| `/` | filter by artist or album name (`Esc` clears) |
| `g` `G` / `Home` `End` / `PgUp` `PgDn` | jump |
| `d` | download the selected album (or all of the artist's albums) and exit the TUI |
| `q` / `Esc` / `Ctrl-C` | quit |

Bubble Tea handles raw mode, mouse and resizing, so it works on Linux, macOS and Windows terminals. Pressing `d` closes the TUI first and then runs the download, so spotdl / yt-dlp / beet get a normal terminal (beet's prompts work).

### Migrating from the shell script

The old `-c` / `-d` JSON files (and `spotdll.json` from the Python version) can be imported:

```sh
mdl import path/to/single-artist.json -a "Artist Name"   # {"Album": "url"}
mdl import path/to/single-artist.json                    # same, artist = name of the file's directory
mdl import path/to/multi-artist.json                     # {"Artist": {"Album": "url"}}
```

The Python script also kept a master log of everything it had downloaded, `~/.spotdll/log.json`, shaped
`{"Artist": {"Album": "url"}}`. Bring it into the library with:

```sh
mdl import --spotdll-log                 # or -l; reads ~/.spotdll/log.json
mdl import -l /path/to/log.json          # if you moved it
mdl -n import -l                         # dry run: see how many albums it would import
```

Years of logging leave odd entries (links that are neither Spotify nor YouTube, albums without an artist), so
this import is forgiving: anything unusable is skipped and listed with the reason, and the rest is imported.
Importing the log twice is harmless; existing albums are updated, not duplicated.

| Old | New |
| --- | --- |
| `script.sh -c` / `-d` (edit JSON) | `mdl -c` (edit + import), `mdl edit`, `mdl add`, `mdl tui` |
| `script.sh -b` | `mdl beet` |
| `script.sh URL` | `mdl URL` |
| `script.sh -s URL`, `-y URL` | `mdl -s URL`, `mdl -y URL` |
| `script.sh -b URL` | `mdl -b URL` |
| (new) file a random URL under Artist/Album | `mdl -t URL` |
| `spotdll.py` with a `spotdll.json` | `mdl` with a `mdl.json` (`mdl -f FILE`, `-d` to delete it afterwards) |
| `spotdll.py -c` | `mdl -c` |
| `~/.spotdll/log.json` | `mdl import --spotdll-log` |
| `.source` file | removed: use `mdl download URL...` (or a shell alias) |

## Development

First time after unpacking: `go mod tidy` (fetches Bubble Tea and Lip Gloss and writes `go.sum`), then commit `go.mod` and `go.sum`.

```sh
make help         # all targets
make check        # gofmt, go mod tidy check, go vet (Linux + Windows), race tests, coverage gate - what CI runs
make cover-html   # coverage report
make dist         # release archives for every platform in ./dist
make tag TAG=v1.0.0   # run checks, tag and push; CI publishes the release
```

Layout:

```
cmd/mdl                 main: wires signals and version
internal/cli            option parser, help text, commands
internal/library        the JSON data store (load, save, merge, import)
internal/downloader     planning, folder layout, spotdl / yt-dlp / beet commands
internal/deps           dependency checks and installation
internal/tui            Bubble Tea model and Lip Gloss rendering (pure model, tested with messages)
internal/runner         process runner interface, dry-run and test fake
```

External programs are always run through `runner.Runner`, which is why nearly everything is unit tested
without downloading anything (`mdl -n ...` uses the same seam for dry runs).

### CI

* **On every pull request** (`.github/workflows/ci.yml`): format check, `go mod tidy` check, `go vet` for Linux and
  Windows, race-enabled tests with an 85 % coverage gate on Linux and macOS, `golangci-lint` (configured in
  `.golangci.yml`), and a cross-compile of every release target. Run the same checks locally with `make check`
  and `make lint`.
* **On a `vX.Y.Z` tag** (`.github/workflows/release.yml`): the same checks, then archives for Linux, macOS and
  Windows plus `checksums.txt` are built with `make dist` and published as a GitHub release with generated
  release notes. A tag with a suffix (`v1.2.3-rc1`) is marked as a pre-release. Cut one with
  `make tag TAG=v1.2.3`.
* **Dependabot** (`.github/dependabot.yml`): every Monday it opens one grouped pull request for Go module updates
  (minor and patch) and one for GitHub Actions updates, which CI then tests like any other pull request.

### Module path

The module is named `mdl`. To `go install` it from GitHub, rename it first:

```sh
go mod edit -module github.com/YOU/mdl
grep -rl '"mdl/' --include='*.go' . | xargs sed -i 's#"mdl/#"github.com/YOU/mdl/#'
```

## Features to build

- [x] Dependabot and CI.
- [x] Choose default editor, not just vi
- [x] Generate an edit file with -c. Import with and without artist defined
  - [x] The default behaviour for running mdl without arguments if a mdl.json is present, is to download on it.
  - [x] The flags to force Youtube or Spotify should be respected.
  - [x] otherwise mdl should use the correct tool depending on the url, and switch if urls are mixed
- [x] -m should use the targetted path for import data.
- [x] Import for old spotdll
- [ ] Implement for easy go distribution
