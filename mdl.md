# MDL

## Args

- [ ] `-c` Open or generate a json file which will allow the batch downloading of files, all the desired albums of an artist.
- [ ] `-d` Open or generate a json file which will allow the batch downloading of files, handling multiple artists and their desired albums.
- [ ] `-s` Force using the spotdl downloader.
- [ ] `-y` Force using the yt-dlp downloader.
- [ ] `-b` Run beet on the current directory, or on each directory in the list

## Idea

The user is either yt or spot links can be called with the same script. It should be handled automatically based on the URL link. You can also specify a json file which can have albums, or even artists with albums, this should be handled automatically.

Directory creation should be handled automatically also.

All events should ultimately be recorded in a master json file.

It would also be good some defaults can be set in a .config file and it should automatically check if all requirements are installed - giving clear instructions for install.

## JSON Examples

```json
{
  "album1": "http://example.com/1",
  "ablum2": "http://example.com/2"
}
```

or

```json
{
  "artist1": {
    "album1": "http://example.com/1",
    "album2": "http://example.com/2"
  },
  "artist2": {
    "album3": "http://example.com/3",
    "album4": "http://example.com/4"
  }
}
```
