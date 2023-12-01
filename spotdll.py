#!/usr/bin/python3

import argparse
import json
from os import getcwd, remove, makedirs, mkdir
from os.path import expanduser, exists, dirname
from subprocess import run
from shutil import copy
from sys import exit

ap = argparse.ArgumentParser()
ap.add_argument("-c", "--create-json", required=False, action="store_true", help="Rather than download albums, generate a spotdll.json file")
ap.add_argument("-s", "--set-json-path", required=False, help="Define a seperate path for the JSON file.")
ap.add_argument("-d", "--delete-json", required=False, action="store_true", help="Deletes the JSON file, after the process is complete.")
ap.add_argument("url", nargs='?', help="URL for single item direct download")

args = vars(ap.parse_args())

class Logger():
    """Simple Spotdll logger.
    """
    def __init__(self, artist: str, data: dict) -> None:
        """Class constructor.

        Args:
            artist (str): The artist being downloaded.
            data (dict): The album & URLs being logged.
        """
        self.artist = artist
        self.data = data
        self.file = None
        self.log_file = expanduser('~/.spotdll/log.json')
        self._create_log_if_not_exists()


    def _create_log_if_not_exists(self) -> None:
        """Create the log file if it doesn't exist.
        """
        directory = dirname(self.log_file)
        if not exists(directory):
            makedirs(directory)
        self.file = open(self.log_file, 'w')
        json.dump({}, self.file)
        self._close()


    def _close(self) -> None:
        """Closes the open file `self.file`.
        """
        self.file.close()


    def log(self) -> None:
        """Executes the actual logging process, adding a new entry (or updating if needed) for each entry on the JSON.
        """
        self.file = open(self.log_file, 'r')
        existing_log = json.load(self.file)
        self._close()
        if self.artist not in existing_log:
            existing_log[self.artist] = {}
        for album, url in self.data.items():
            existing_log[self.artist][album] = url
        self.file = open(self.log_file, 'a')
        json.dump(existing_log, self.file, indent=2)
        self._close()

class Spotdll():
    """Perform a batch spotdl task.
    """
    def __init__(self, args: dict) -> None:
        """Class constructor

        Args:
            args (dict): The arguments parsed to the script.
        """
        self.args = args
        self.delete_when_complete = False

        self.sample_json = expanduser('~/bin/apps/ubiquitous-sniffle/spotdll-sample.json')

        self.cwd = getcwd()
        self.json_path = expanduser(self.cwd + "/spotdll.json")

        self._handle_arguments()

        self.data = self.read_validate_json()

        for album, url in self.data.items():
            self.execute_download(album, url)

        log = Logger(self.cwd.split('/')[-1], self.data)
        log.log()

        if self.delete_when_complete is True:
            remove(self.json_path)


    def _handle_arguments(self) -> None:
        """Goes through parsed atguments and handle as needed.
        """
        if self.args["create_json"] is True:
            self.generate_json_file()

        if self.args["delete_json"] is True:
            self.delete_when_complete = True

        if self.args["set_json_path"] is not None:
            self.json_path = self.args["set_json_path"]

        if self.args['url'] is not None:
            self.execute_direct_download(self.args['url'])
            self.exit("Album " + self.args['url'] + " downloaded.")


    def generate_json_file(self) -> None:
        """Generate a JSON file in the CWD as required. Will not overwrite the
        file if it already exists
        """
        if exists(self.json_path):
            self.exit("JSON file " + self.json_path + " already exists, cancelling creation.")
        copy(self.sample_json, self.json_path)
        run(['nano', self.json_path])
        self.exit("JSON file " + self.json_path + " has been created.")


    def exit(self, closing_message: str = None) -> None:
        """A tool to kill the script immediately.

        Args:
            closing_message (str, optional): An optional closing message
            to be printed. Defaults to None.
        """
        if closing_message is not None:
            print(closing_message)
        exit()


    def read_validate_json(self) -> dict|list:
        """Reads and validates the spotdll.json file. If the file doesn't exist
        or is invalid, an error is shown and the script exits gracefully.
        Otherwise reads the json to `self.data`.

        Raises:
            `json.decoder.JSONDecodeError`: If the JSON file is invalid.

        Returns:
            dict|list: The validated JSON data.
        """
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
            album (str): The name of the album being downloaded. A dir of this 
                         name is created in the CWD and the songs are downloaded into it.
            url (str): The spotify URL of the album, playlist, song etc.
        """
        print("\nDownloading album: " + album.upper() + "\n")
        wd = self.cwd + '/' + album
        mkdir(wd)
        execute_direct_download(url, wd)
        run(['spotdl', 'download', url], cwd=wd)

    def execute_direct_download(self, url: str, directory: str = None) -> None:
        """Executes a direct command to download an album from a parsed URL

        Args:
            url (str): The URL to download from.
            directory (str, optional): what directory to execute the command in.
        """
        if directory is not None:
            run(['spotdl', 'download', url], cwd=directory)
        else:
            run(['spotdl', 'download', url])

spotdll = Spotdll(args)
