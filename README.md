# ubiquitous-sniffle

![GitHub](https://img.shields.io/github/license/projector22/ubiquitous-sniffle) ![GitHub last commit](https://img.shields.io/github/last-commit/projector22/ubiquitous-sniffle) ![GitHub release (latest by date)](https://img.shields.io/github/v/release/projector22/ubiquitous-sniffle)

A batch tool for using [spotDL](https://github.com/spotDL/spotify-downloader) to execute a number of tasks synchronously.

Create a JSON file in the artist's directory then execute `spotdll` to perform the action.

## Arguments

- `-c`, `--create-json` Rather than download albums, generate a spotdll.json file.
- `-s`, `--set-json-path` Define a seperate path for the JSON file.
- `-d`, `--delete-json` Deletes the JSON file, after the process is complete.

## Requirements

- `spotdl`
