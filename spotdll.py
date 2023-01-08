#!/usr/bin/python3

import argparse

ap = argparse.ArgumentParser()
ap.add_argument("-c", "--create-json", required=False, action="store_true", help="Rather than download albums, generate a spotdll.json file")
ap.add_argument("-s", "--set-json-path", required=False, help="Define a seperate path for the JSON file.")
ap.add_argument("-d", "--delete-json", required=False, action="store_true", help="Deletes the JSON file, after the process is complete.")

args = vars(ap.parse_args())

class Spotdll():

    def __init__(self, args: dict) -> None:
        """Class constructor

        Args:
            args (dict): The arguments parsed to the script.
        """
        from os import getcwd
        from os.path import expanduser        

        self.args = args
        self.delete_when_complete = False
        
        self.sample_json = expanduser('~/bin/apps/ubiquitous-sniffle/spotdll-sample.json')
        
        self.cwd = getcwd()
        self.json_path = expanduser(self.cwd + "/spotdll.json")

        self._handle_arguments()

        self.data = self.read_validate_json()

        for album, url in self.data.items():
            self.execute_download(album, url)

        if self.delete_when_complete == True:
            from os import remove
            remove(self.json_path)


    def _handle_arguments(self) -> None:
        """Goes through parsed atguments and handle as needed.
        """
        if self.args["create_json"] == True:
            self.generate_json_file()

        if self.args["delete_json"] == True:
            self.delete_when_complete = True

        if self.args["set_json_path"] != None:
            self.json_path = self.args["set_json_path"]


    def generate_json_file(self) -> None:
        """Generate a JSON file in the CWD as required. Will not overwrite the file if it already exists
        """
        from os.path import exists
        if exists(self.json_path):
            self.exit("JSON file " + self.json_path + " already exists, cancelling creation.")
        from shutil import copy
        copy(self.sample_json, self.json_path)
        self.exit("JSON file " + self.json_path + " has been created.")


    def exit(self, closing_message: str = None) -> None:
        """A tool to kill the script immediately.

        Args:
            closing_message (str, optional): An optional closing message to be printed. Defaults to None.
        """
        if closing_message is not None:
            print(closing_message)
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
            self.exit(self.json_path + " doesn't exist. There is nothing to download therefore the task cannot continue.")
        try:
            file = open(self.json_path, 'r')
            data = json.load(file)
            if not isinstance(data, dict) and not isinstance(data, list):
                raise json.decoder.JSONDecodeError()
            return data
        except json.decoder.JSONDecodeError:
            self.exit("Invalid data in spotdll.json")
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
