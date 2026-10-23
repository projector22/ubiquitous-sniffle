# mdl

Download, organise and browse a personal music library from the terminal.

`mdl` keeps **one library file** mapping *artist → album → Spotify / YouTube link*,
downloads those albums with [spotdl](https://github.com/spotDL/spotify-downloader)
or [yt-dlp](https://github.com/yt-dlp/yt-dlp) into `<music-dir>/<Artist>/<Album>/`,
and can hand the result to [beets](https://beets.io) for tagging.

It is a single static binary. The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); everything else uses only the standard library.

## Install

```sh
make install              # builds and copies to /usr/local/bin (PREFIX=~/.local to change)
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

`mdl URL...` is shorthand for `mdl download URL...`. The tool is picked from the link (Spotify → spotdl,
YouTube → yt-dlp); `-s`/`--spotify` and `-y`/`--youtube` override that. The older names `--spotdl` and
`--yt-dlp` still work. Use `-m DIR` to download somewhere else.

For a managed library:

```sh
mdl add -a "Daft Punk" -A Discovery -u https://open.spotify.com/album/...
mdl add -a "Daft Punk" -A Discovery -u https://music.youtube.com/playlist?list=...   # adds the YouTube link too
mdl list -u                       # tree of everything (-u also prints links)
mdl tui                           # browse; press d to download the selection
mdl download                      # every album in the library
mdl download -a "Daft Punk" -sb   # one artist, force Spotify links, then run beet on each folder
mdl download --youtube            # use each album's YouTube link instead
mdl beet ~/Music/Daft\ Punk       # run beet on a folder (default: current directory)
mdl edit                          # open the library in $EDITOR
mdl -n download                   # dry run: show the commands, change nothing
```

Run `mdl --help` for the full reference, or `mdl <command> --help`.

Every option has a short and a long form (`-s`/`--spotdl`, `-b`/`--beet`, `-a`/`--artist`, …),
short flags combine (`-sb`), and values can be attached (`--artist=NAME`, `-aNAME`).

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
Music directory for the `Artist/Album` layout: `-m DIR`, else `$MDL_MUSIC_DIR`, else the current directory. Plain URL downloads ignore `$MDL_MUSIC_DIR` and use the current directory unless `-m` is given.

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

The old `-c` / `-d` JSON files can be imported:

```sh
mdl import path/to/single-artist.json -a "Artist Name"   # {"Album": "url"}
mdl import path/to/multi-artist.json                     # {"Artist": {"Album": "url"}}
```

| Old | New |
| --- | --- |
| `script.sh -c` / `-d` (edit JSON) | `mdl edit`, `mdl add`, `mdl tui` |
| `script.sh -b` | `mdl beet` |
| `script.sh URL` | `mdl URL` |
| `script.sh -s URL`, `-y URL` | `mdl -s URL`, `mdl -y URL` |
| `script.sh -b URL` | `mdl -b URL` |
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

* **On every commit / PR** (`.github/workflows/ci.yml`): format check, `go mod tidy` check, `go vet` for Linux and Windows,
  race-enabled tests with an 85 % coverage gate on Linux and macOS, `golangci-lint`, then a full
  cross-platform build whose archives are uploaded as workflow artifacts.
* **On a `vX.Y.Z` tag** (`.github/workflows/release.yml`): the same checks, then archives and
  `checksums.txt` are published as a GitHub release.

### Module path

The module is named `mdl`. To `go install` it from GitHub, rename it first:

```sh
go mod edit -module github.com/YOU/mdl
grep -rl '"mdl/' --include='*.go' . | xargs sed -i 's#"mdl/#"github.com/YOU/mdl/#'
```
