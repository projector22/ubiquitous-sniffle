#!/usr/bin/python3

import argparse

ap = argparse.ArgumentParser()
ap.add_argument("-c", "--create-json", required=False, action="store_true", help="Rather than download albums, generate a spotdll.json file")

args = vars(ap.parse_args())

class Spotdll():

    def __init__(self, args):
        from os import getcwd
        from os.path import expanduser
        self.cwd = getcwd()
        self.json_path = expanduser(self.cwd + "/spotdll.json")
        self.data = self.read_validate_json()
        for album, url in self.data.items():
            self.execute_download(album, url)
        
        # print(self.data)

    def exit(self) -> None:
        """Kills the script as required.
        """
        import sys
        sys.exit()


    def read_validate_json(self) -> dict|list:
        """Reads and validates the spotdll.json file. If the file doesn't exist or is invalid, an error is shown and
        the script exits gracefully. Otherwise reads the json to `self.data`.

        Raises:
            `json.decoder.JSONDecodeError`: If the JSON file is invalid.

        Returns:
            dict|list: The validated JSON data.
        """
        import json
        from os.path import exists
        if not exists(self.json_path):
            print(self.json_path + " doesn't exist. There is nothing to download therefore the task cannot continue.")
            self.exit()
        try:
            file = open(self.json_path, 'r')
            data = json.load(file)
            if not isinstance(data, dict) and not isinstance(data, list):
                raise json.decoder.JSONDecodeError()
            return data
        except json.decoder.JSONDecodeError:
            print("Invalid data in spotdll.json")
            self.exit()
        finally:
            file.close()


    def execute_download(self, album: str, url: str) -> None:
        """Execute the download of an album, playlist, song etc.

        Args:
            album (str): The name of the album being downloaded. A dir of this name is created in the CWD and the songs are downloaded into it.
            url (str): The spotify URL of the album, playlist, song etc.
        """
        from subprocess import run
        from os import mkdir
        print("\nDownloading album: " + album.upper() + "\n")
        wd = self.cwd + '/' + album
        mkdir(wd)
        run(['spotdl', 'download', url], cwd=wd)

        
        



spotdll = Spotdll(args)

# # Loop through the album names and URLs in the JSON data
# for album, url in data.items():
#     # Use subprocess to call the spotdl command with the URL

#     