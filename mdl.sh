#!/bin/bash


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

URL="$1"

exec_download "$URL"