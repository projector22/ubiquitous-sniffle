# Ubiquitous Sniffle

## v0.2.2 - 2023-02-01

## Added

- Added a nano edit on creating the json

### Fixed

- Fixed log creation.

## Changed

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
