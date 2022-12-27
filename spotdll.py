#!/usr/bin/python3

import json
import subprocess
import os

OUTPUT = os.getcwd()
JSON = "spotdll.json"

# Load the JSON data from the file
with open(JSON, 'r') as f:
    data = json.load(f)

# Loop through the album names and URLs in the JSON data
for album, url in data.items():
    # Use subprocess to call the spotdl command with the URL
    print("\nDownloading album: " + album.upper() + "\n")
    wd = OUTPUT + '/' + album
    os.mkdir(wd)
    subprocess.run(['spotdl', 'download', url], cwd=wd)