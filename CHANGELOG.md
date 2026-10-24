# Ubiquitous Sniffle

## Unreleased

### Added

- GitHub Actions: `ci.yml` tests every pull request (format, `go mod tidy`, vet, race tests with coverage gate,
  `golangci-lint`, cross-compile); `release.yml` tests, builds and publishes a GitHub release for every `vX.Y.Z` tag.
- Dependabot for Go modules and GitHub Actions (weekly, grouped).
- `.golangci.yml` for golangci-lint v2.
- `mdl -c` / `mdl import -c [FILE]`: create (or edit, if it exists) a batch file from a sample in your editor, then
  import it into the library. Modelled on `spotdll.py -c`. Saving the sample untouched imports nothing.
- `mdl import -d` / `--delete`: remove the batch file after a successful import (like `spotdll.py -d`).

### Changed

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
