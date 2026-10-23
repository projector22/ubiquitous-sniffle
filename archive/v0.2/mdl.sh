#!/bin/bash

function show_help() {
    cat <<'EOF'
Usage:
  script.sh [OPTIONS] [URL ...]

Options:
  -c    Open or generate a JSON file for batch downloading albums for a
        single artist. Config/editor mode only; does not download.

  -d    Open or generate a JSON file for batch downloading albums for
        multiple artists. Config/editor mode only; does not download.

  -s    Force use of the spotdl downloader.

  -y    Force use of the yt-dlp downloader.

  -b    Run beet:
          - on the current directory when used on its own
          - on each downloaded directory when used with a download run

  -h    Show this help and exit.

Rules:
  - -c and -d are mutually exclusive.
  - -c and -d may not be used with -b.
  - -s and -y are mutually exclusive.

Default behaviour:
  If no flags and no URL/source arguments are provided, the script will:
    1. attempt to download from the generated JSON file
    2. otherwise execute the .source file if present
    3. otherwise show this help
EOF
}

function single_template() {
    cat <<'EOF'
{
    "SAMPLE_ALBUM_NAME": "https://open.spotify.com/album/your_custom_spotify_url"
}
EOF
}

function multi_template() {
    cat <<'EOF'
{
    "SAMPLE_ARTIST": {
        "SAMPLE_ALBUM_NAME": "https://open.spotify.com/album/your_custom_spotify_url"
    }
}
EOF
}

#
# Configurable paths
#
SINGLE_JSON="./mdl.json"
MULTI_JSON="./mdl.json"
DEFAULT_JSON="./mdl.json"
SOURCE_FILE="./.source"

CONFIG_MODE=""
DOWNLOADER=""
RUN_BEET=0
TEXT_EDITOR="nvim"

while getopts "cdsybh" opt; do
    case "$opt" in
        c)
            if [ -n "$CONFIG_MODE" ]; then
                echo "Error: only one of -c or -d may be specified." >&2
                exit 1
            fi
            CONFIG_MODE="single"
            ;;
        d)
            if [ -n "$CONFIG_MODE" ]; then
                echo "Error: only one of -c or -d may be specified." >&2
                exit 1
            fi
            CONFIG_MODE="multi"
            ;;
        s)
            if [ -n "$DOWNLOADER" ]; then
                echo "Error: only one of -s or -y may be specified." >&2
                exit 1
            fi
            DOWNLOADER="spotdl"
            ;;
        y)
            if [ -n "$DOWNLOADER" ]; then
                echo "Error: only one of -s or -y may be specified." >&2
                exit 1
            fi
            DOWNLOADER="yt-dlp"
            ;;
        b)
            RUN_BEET=1
            ;;
        h)
            show_help
            exit 0
            ;;
        \?)
            show_help >&2
            exit 1
            ;;
    esac
done

shift $((OPTIND - 1))

#
# Remaining args are treated as explicit download targets, e.g. URLs
#
HAS_TARGETS=0
if [ "$#" -gt 0 ]; then
    HAS_TARGETS=1
fi

#
# Validation
#

# -c / -d are editor-only and cannot be combined with -b
if [ -n "$CONFIG_MODE" ] && [ "$RUN_BEET" -eq 1 ]; then
    echo "Error: -b may not be used with -c or -d." >&2
    exit 1
fi

#
# Stub functions - replace these with your real implementations
#

function edit_single_json() {
    echo "Open/generate single-artist JSON: $SINGLE_JSON"

    if [ ! -f "$SINGLE_JSON" ]; then
        touch $SINGLE_JSON
        single_template >> $SINGLE_JSON
    fi
    $TEXT_EDITOR $SINGLE_JSON
}

function edit_multi_json() {
    echo "Open/generate multi-artist JSON: $MULTI_JSON"
    if [ ! -f "$SINGLE_JSON" ]; then
        touch $MULTI_JSON
        multi_template >> $MULTI_JSON
    fi
    $TEXT_EDITOR $MULTI_JSON
}

function spot_download {
    spotdl --bitrate 128k download $1
}

function yt_download {
    yt-dlp \
      --no-warnings \
      --progress \
      -f "bestaudio/best" \
      -x \
      --audio-format mp3 \
      --audio-quality 0 \
      --embed-metadata \
      --embed-thumbnail \
      -o "%(playlist_index)02d - %(artist)s - %(title)s.%(ext)s" \
      --js-runtimes node \
      "$1"
}

function exec_download {
    local DL_URL="$1"
    local YT_URL="music.youtube"
    case "$string" in
        *"$YT_URL"*)
            yt_download "$DL_URL"
            ;;
        *)
            spot_download "$DL_URL"
            ;;
    esac
}

function download_from_json() {
    json_file=$1
    downloader=$2


    echo "Download from JSON: $json_file"
    if [ -n "$downloader" ]; then
        echo "Downloader forced: $downloader"
    fi

    # Return list of downloaded directories somehow if needed by beet
    jq -r 'to_entries[] | [.key, .value] | @tsv' "$json_file" |
    while IFS='	' read -r album_name spotify_url; do
        create_dir_and_download "$album_name" "$spotify_url"
    done
}

function download_from_args() {
    downloader=$1
    shift
    # echo "Download from explicit target: $*"
    if [ -n "$downloader" ]; then
        echo "Downloader forced: $downloader"
	case "$downloader" in
		spotdl) spot_download "$*";;
		yt-dlp) yt_download "$*" ;;
	esac
    else
	exec_download "$*"
    fi


}

function run_source_file() {
    file=$1
    echo "Executing source file: $file"
    # shellcheck disable=SC1090
    . "$file"
}

function run_beet_current_dir() {
    echo "Running beet on current directory"
    beet import -C .
}

function run_beet_on_downloads() {
    echo "Running beet on downloaded directories"

    # Currently it's only the current working directory.
   run_beet_current_dir
}

function create_dir_and_download() {
	local ALBUM_NAME="$1"
	local ALBUM_URL="$2"

	mkdir -p "$ALBUM_NAME"
	cd "$ALBUM_NAME"
	exec_download "$ALBUM_URL"
}

#
# Dispatch
#

# 1) Config/editor mode always wins
if [ -n "$CONFIG_MODE" ]; then
    case "$CONFIG_MODE" in
        single) edit_single_json ;;
        multi)  edit_multi_json ;;
    esac
    exit 0
fi

# 2) Beet-only mode: -b with no explicit download targets and no forced downloader
#    This means "apply beet to current directory"
if [ "$RUN_BEET" -eq 1 ] && [ "$HAS_TARGETS" -eq 0 ] && [ -z "$DOWNLOADER" ]; then
    run_beet_current_dir
    exit 0
fi

# 3) Explicit download targets provided
if [ "$HAS_TARGETS" -eq 1 ]; then


    download_from_args "$DOWNLOADER" "$@"

    if [ "$RUN_BEET" -eq 1 ]; then
        run_beet_on_downloads
    fi
    exit 0
fi

# 4) Downloader forced without explicit targets:
#    treat as a request to perform the default download flow
if [ -n "$DOWNLOADER" ]; then
    if [ -f "$DEFAULT_JSON" ]; then
        download_from_json "$DEFAULT_JSON" "$DOWNLOADER"
        if [ "$RUN_BEET" -eq 1 ]; then
            run_beet_on_downloads
        fi
        exit 0
    fi

    if [ -f "$SOURCE_FILE" ]; then
        run_source_file "$SOURCE_FILE"
        if [ "$RUN_BEET" -eq 1 ]; then
            run_beet_on_downloads
        fi
        exit 0
    fi

    echo "Error: no download targets, JSON config, or .source file found." >&2
    show_help >&2
    exit 1
fi

# 5) No flags and no targets:
#    JSON -> .source -> help
if [ -f "$DEFAULT_JSON" ]; then
    download_from_json "$DEFAULT_JSON" ""
    exit 0
fi

if [ -f "$SOURCE_FILE" ]; then
    run_source_file "$SOURCE_FILE"
    exit 0
fi

show_help
exit 1
