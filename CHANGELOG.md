# Ubiquitous Sniffle

## Unreleased

### Added

- Bandcamp as an audio source for spotdl: `mdl config audio "bandcamp youtube-music"` or `-p` / `--audio LIST` on
  `download`, `tui` and plain `mdl` passes `--audio` to spotdl, so Spotify links are matched to Bandcamp audio first,
  falling back to the next provider. Also accepts `youtube`, `soundcloud` and `piped`.
- `mdl tui`: `x` / `Delete` removes the selected album, or the artist with all its albums, from the library after
  a y/n confirmation, and saves the library immediately. Files already downloaded are not touched.
- Plain `mdl` downloads `./mdl.json` when it exists (the original script's normal mode); `mdl -f FILE` / `mdl download
  --file FILE` names another batch file and `-d` / `--delete` removes it after a fully successful run. A single-artist
  file downloads into `<dir>/<Album>/` (the directory is the artist's folder), a multi-artist file into
  `<dir>/<Artist>/<Album>/`. Albums download in file order, each URL with the tool for its host (Spotify and YouTube
  may be mixed), `--spotify` / `--youtube` force one, and every album that succeeds is recorded in the library.
- `mdl import --spotdll-log` (`-l`) imports the old Python script's `~/.spotdll/log.json`, skipping and listing
  entries that can no longer be used instead of failing.
- Plain `mdl import` imports `./mdl.json`.
- Clear "nothing to download" message for download options given with no URL and no `mdl.json` (previously "unknown option").
- `mdl config [KEY [VALUE]]` and a config file (`~/.config/mdl/config.json`, or `-C` / `$MDL_CONFIG`), kept apart
  from the library. The first setting is `editor`: the command used by `mdl edit` and `mdl -c`, with arguments and
  quoting (`mdl config editor "code --wait"`). `--unset` removes a setting.
- `--editor` / `-e` on `mdl import` (for `-c`), as `mdl edit` already had.
- GitHub Actions: `ci.yml` tests every pull request (format, `go mod tidy`, vet, race tests with coverage gate,
  `golangci-lint`, cross-compile); `release.yml` tests, builds and publishes a GitHub release for every `vX.Y.Z` tag.
- Dependabot for Go modules and GitHub Actions (weekly, grouped).
- `.golangci.yml` for golangci-lint v2.
- `mdl -c` / `mdl import -c [FILE]`: create (or edit, if it exists) a batch file from a sample in your editor, then
  import it into the library. Modelled on `spotdll.py -c`. Saving the sample untouched imports nothing.
- `mdl import -d` / `--delete`: remove the batch file after a successful import (like `spotdll.py -d`).

### Changed

- `-m DIR` is now the target directory for batch files (`mdl.json` is read from and downloaded into it; `mdl import`
  and `mdl -c` use `DIR/mdl.json`) and the directory whose name decides the artist and album a plain URL is recorded
  under. `$MDL_MUSIC_DIR` is still only the root of the `Artist/Album` layout.
- Batch files keep the order of their entries.
- The editor is chosen from `--editor`, then the config file, then `$VISUAL`, `$EDITOR` and finally `nano`. Help and
  docs no longer mention `vi` as the default.
- `mdl import` of a single-artist file (`{"Album": "url"}`) no longer requires `--artist`: without it the albums
  are filed under the name of the directory containing the file.

### Fixed

- A test no longer leaves an `internal/cli/Daft Punk` directory behind in the source tree.

## v0.2.4 - 2024-06-03

### Added

- NTFY push notifications.

### Changed

- Changed how the -c switch works. It now allows editing of the file if it already exists.

### Fixed

- Fixed a crash when the directory exists when going through a json object.
- Fixed log writing.

## v0.2.3 - 2023-02-01

### Fixed

- Fixed import for `makedirs`

---

## v0.2.2 - 2023-02-01

### Added

- Added a nano edit on creating the json

### Fixed

- Fixed log creation.

### Changed

- Changed log path to ~/.spotdll/log.json

---

## v0.2.1 - 2023-01-08

## Changed

- Changed location of log to home folder

---

## v0.2.0 - 2023-01-08

### Added

- Added a switch to generate the `spotdll.json` file from sample in the Current working directory. #3
- Added a switch to define the JSON file to a completely separate and unrelated path and file to the standard behaviour. #3
- Added a switch to instruct the script to delete the JSON file after the task is complete. #3
- Added logging task. #4

### Changed

- The location of the file `spotdll.json` is now called from the folder the script is being called from, rather than from `~/bin`. #1

### Issues Closed

- #1
- #3
- #4

---

## v0.1.0

- Added basic tooling.
