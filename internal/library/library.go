// Package library implements mdl's single, human-readable data store.
//
// The store is one JSON document shaped artist -> album -> links:
//
//	{
//	  "Daft Punk": {
//	    "Discovery": {
//	      "spotify": "https://open.spotify.com/album/...",
//	      "youtube": "https://music.youtube.com/playlist?list=..."
//	    }
//	  }
//	}
//
// Keys are written in sorted order with two-space indentation so the file
// diffs cleanly and is pleasant to edit by hand.
package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mdl/internal/atomicfile"
)

// Entry holds the download links for one album. At least one link must be set
// for the entry to be downloadable.
type Entry struct {
	Spotify string `json:"spotify,omitempty"`
	YouTube string `json:"youtube,omitempty"`
}

// Empty reports whether the entry has no links at all.
func (e Entry) Empty() bool { return e.Spotify == "" && e.YouTube == "" }

// Source identifies which service a URL belongs to.
type Source int

// Known URL sources.
const (
	SourceUnknown Source = iota
	SourceSpotify
	SourceYouTube
)

// String returns a lower-case name for the source.
func (s Source) String() string {
	switch s {
	case SourceSpotify:
		return "spotify"
	case SourceYouTube:
		return "youtube"
	}
	return "unknown"
}

// ClassifyURL decides whether raw is a Spotify or YouTube (incl. YouTube
// Music) link. Anything else is an error.
func ClassifyURL(raw string) (Source, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return SourceUnknown, fmt.Errorf("%q is not a valid http(s) URL", raw)
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "spotify.com" || strings.HasSuffix(host, ".spotify.com"):
		return SourceSpotify, nil
	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be":
		return SourceYouTube, nil
	}
	return SourceUnknown, fmt.Errorf("%q is neither a Spotify nor a YouTube link", raw)
}

// SetURL classifies raw and stores it in the matching field of e.
func (e *Entry) SetURL(raw string) error {
	src, err := ClassifyURL(raw)
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if src == SourceSpotify {
		e.Spotify = raw
	} else {
		e.YouTube = raw
	}
	return nil
}

// Library is an in-memory artist -> album -> Entry mapping.
type Library struct {
	data map[string]map[string]Entry
}

// New returns an empty library.
func New() *Library { return &Library{data: map[string]map[string]Entry{}} }

// DefaultPath returns where the library lives when -L is not given:
// $MDL_LIBRARY, else $XDG_CONFIG_HOME/mdl/library.json, else
// <home>/.config/mdl/library.json.
func DefaultPath(getenv func(string) string, home string) string {
	if p := getenv("MDL_LIBRARY"); p != "" {
		return p
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mdl", "library.json")
	}
	if home == "" {
		return "library.json"
	}
	return filepath.Join(home, ".config", "mdl", "library.json")
}

// Load reads the library at path. A missing file yields an error satisfying
// errors.Is(err, fs.ErrNotExist); use LoadOrNew when that is acceptable.
func Load(path string) (*Library, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	lib, err := Read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lib, nil
}

// LoadOrNew is like Load but returns an empty library if the file is absent.
func LoadOrNew(path string) (*Library, error) {
	lib, err := Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	return lib, err
}

// Read decodes a library from r. Unknown fields are rejected so typos such as
// "youtbe" are caught instead of silently ignored. An empty document is an
// empty library.
func Read(r io.Reader) (*Library, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var data map[string]map[string]Entry
	if err := dec.Decode(&data); err != nil {
		if errors.Is(err, io.EOF) {
			return New(), nil
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, fmt.Errorf("invalid library: %w (is this an old-style mdl.json? try `mdl import FILE`)", err)
		}
		return nil, fmt.Errorf("invalid library: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid library: unexpected data after the top-level object")
	}
	if data == nil {
		data = map[string]map[string]Entry{}
	}
	for artist, albums := range data {
		if strings.TrimSpace(artist) == "" {
			return nil, errors.New("invalid library: empty artist name")
		}
		if albums == nil {
			data[artist] = map[string]Entry{}
		}
		for album := range albums {
			if strings.TrimSpace(album) == "" {
				return nil, fmt.Errorf("invalid library: empty album name under %q", artist)
			}
		}
	}
	return &Library{data: data}, nil
}

// Write encodes the library as indented JSON. HTML escaping is disabled so
// "&" in URLs stays readable.
func (l *Library) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(l.data)
}

// Save writes the library to path atomically (temp file + rename), creating
// parent directories as needed.
func (l *Library) Save(path string) error {
	return atomicfile.Write(path, 0o644, l.Write)
}

// ResolveArtist finds an existing artist by exact name, falling back to a
// case-insensitive match. It returns the canonical stored spelling.
func (l *Library) ResolveArtist(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if _, ok := l.data[name]; ok {
		return name, true
	}
	for a := range l.data {
		if strings.EqualFold(a, name) {
			return a, true
		}
	}
	return "", false
}

// ResolveAlbum is ResolveArtist for albums under a canonical artist name.
func (l *Library) ResolveAlbum(artist, album string) (string, bool) {
	album = strings.TrimSpace(album)
	albums := l.data[artist]
	if _, ok := albums[album]; ok {
		return album, true
	}
	for a := range albums {
		if strings.EqualFold(a, album) {
			return a, true
		}
	}
	return "", false
}

// Add inserts or updates an album. Existing artists/albums are matched
// case-insensitively; non-empty links in e overwrite stored ones while links
// left empty in e are preserved.
func (l *Library) Add(artist, album string, e Entry) error {
	artist, album = strings.TrimSpace(artist), strings.TrimSpace(album)
	switch {
	case artist == "":
		return errors.New("artist name is required")
	case album == "":
		return errors.New("album name is required")
	case e.Empty():
		return errors.New("at least one Spotify or YouTube link is required")
	}
	if a, ok := l.ResolveArtist(artist); ok {
		artist = a
	}
	if l.data[artist] == nil {
		l.data[artist] = map[string]Entry{}
	}
	if a, ok := l.ResolveAlbum(artist, album); ok {
		album = a
	}
	cur := l.data[artist][album]
	if e.Spotify != "" {
		cur.Spotify = e.Spotify
	}
	if e.YouTube != "" {
		cur.YouTube = e.YouTube
	}
	l.data[artist][album] = cur
	return nil
}

// Merge adds every album of other into l and returns how many albums were
// processed.
func (l *Library) Merge(other *Library) (int, error) {
	n := 0
	for _, artist := range other.Artists() {
		for _, album := range other.Albums(artist) {
			e, _ := other.Get(artist, album)
			if err := l.Add(artist, album, e); err != nil {
				return n, fmt.Errorf("%s / %s: %w", artist, album, err)
			}
			n++
		}
	}
	return n, nil
}

// Get returns the entry for an exact artist/album pair.
func (l *Library) Get(artist, album string) (Entry, bool) {
	e, ok := l.data[artist][album]
	return e, ok
}

// Artists returns all artist names, sorted case-insensitively.
func (l *Library) Artists() []string { return sortedKeys(l.data) }

// Albums returns the album names of an artist, sorted case-insensitively.
func (l *Library) Albums(artist string) []string { return sortedKeys(l.data[artist]) }

// NumArtists returns the number of artists.
func (l *Library) NumArtists() int { return len(l.data) }

// NumAlbums returns the total number of albums across all artists.
func (l *Library) NumAlbums() int {
	n := 0
	for _, albums := range l.data {
		n += len(albums)
	}
	return n
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := strings.ToLower(keys[i]), strings.ToLower(keys[j])
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

// Placeholder values of the template written by `mdl import --create`.
// Entries that still carry SampleURL are ignored on import, so saving the
// template untouched can never add a fake album to the library.
const (
	SampleAlbum = "SAMPLE_ALBUM_NAME"
	SampleURL   = "https://open.spotify.com/album/your_custom_spotify_url"
)

// BatchTemplate returns the starting content of a new batch (edit) file: the
// single-artist form with one placeholder album.
func BatchTemplate() []byte {
	return []byte("{\n  \"" + SampleAlbum + "\": \"" + SampleURL + "\"\n}\n")
}

// BatchEntry is one album from a batch file. Artist is empty for the
// single-artist form ({"Album": "url"}), which does not say whose album it is.
type BatchEntry struct {
	Artist string
	Album  string
	Entry  Entry
}

// Skipped describes something a lenient parse (ParseLog) left out.
type Skipped struct {
	Where, Reason string
}

type member struct {
	key string
	val json.RawMessage
}

// orderedObject decodes a JSON object keeping its members in file order
// (a Go map would lose it, and downloads should run in the order written).
func orderedObject(data []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("expected an object")
	}
	var out []member
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out = append(out, member{kt.(string), raw})
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the top-level object")
	}
	return out, nil
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// parseBatch reads both batch shapes. Strict mode stops at the first problem;
// lenient mode (for old logs) records it in the skipped list and carries on.
// Entries come back in file order. Placeholder entries (SampleURL) are
// dropped silently.
func parseBatch(data []byte, lenient bool) (entries []BatchEntry, skipped []Skipped, err error) {
	top, err := orderedObject(data)
	if err != nil {
		return nil, nil, fmt.Errorf("not a JSON object: %w", err)
	}
	// problem either aborts (strict) or notes and continues (lenient).
	problem := func(where string, e error) error {
		if lenient {
			skipped = append(skipped, Skipped{where, e.Error()})
			return nil
		}
		return fmt.Errorf("%s: %w", where, e)
	}
	add := func(artist, album, rawURL string) error {
		if strings.TrimSpace(rawURL) == SampleURL {
			return nil
		}
		where := album
		if artist != "" {
			where = artist + " / " + album
		}
		var e Entry
		if err := e.SetURL(rawURL); err != nil {
			return problem(where, err)
		}
		entries = append(entries, BatchEntry{Artist: artist, Album: album, Entry: e})
		return nil
	}

	for _, m := range top {
		if isNull(m.val) {
			continue // an artist without albums (Go would also decode null as an empty string)
		}
		var flat string
		if json.Unmarshal(m.val, &flat) == nil {
			// Single-artist form: key is an album, value a URL.
			if lenient && strings.TrimSpace(flat) != SampleURL {
				skipped = append(skipped, Skipped{m.key, "no artist (expected {artist: {album: url}})"})
				continue
			}
			if err := add("", m.key, flat); err != nil {
				return nil, nil, err
			}
			continue
		}
		albums, oerr := orderedObject(m.val)
		if oerr != nil {
			if err := problem(m.key, errors.New("unsupported value")); err != nil {
				return nil, nil, fmt.Errorf("%q: unsupported value", m.key)
			}
			continue
		}
		for _, al := range albums {
			var u string
			if json.Unmarshal(al.val, &u) != nil {
				e := errors.New("expected a URL string; this looks like it is already in the new format (use it directly with --library)")
				if err := problem(m.key+" / "+al.key, e); err != nil {
					return nil, nil, err
				}
				continue
			}
			if err := add(m.key, al.key, u); err != nil {
				return nil, nil, err
			}
		}
	}
	return entries, skipped, nil
}

// ParseBatch reads a batch file in either of its two shapes:
//
//	single artist:  {"Album": "url"}
//	multi artist:   {"Artist": {"Album": "url"}}
//
// (the formats of the old shell and Python versions), in file order.
func ParseBatch(data []byte) ([]BatchEntry, error) {
	entries, _, err := parseBatch(data, false)
	return entries, err
}

// ParseLog reads the old spotdll master log (~/.spotdll/log.json,
// {"Artist": {"Album": "url"}}). Years of logging leave odd entries, so
// anything unusable is returned in skipped instead of failing the import.
func ParseLog(data []byte) (entries []BatchEntry, skipped []Skipped, err error) {
	return parseBatch(data, true)
}

// ImportBatch turns a batch file into a Library. A single-artist file does
// not say whose albums they are, so they are filed under defaultArtist;
// usedDefault reports whether that happened. Placeholder entries (SampleURL)
// are skipped.
func ImportBatch(data []byte, defaultArtist string) (lib *Library, usedDefault bool, err error) {
	entries, err := ParseBatch(data)
	if err != nil {
		return nil, false, err
	}
	lib = New()
	for _, e := range entries {
		artist := e.Artist
		if artist == "" {
			if strings.TrimSpace(defaultArtist) == "" {
				return nil, false, fmt.Errorf("%q maps straight to a URL, so this is a single-artist file: pass --artist", e.Album)
			}
			artist, usedDefault = defaultArtist, true
		}
		if err := lib.Add(artist, e.Album, e.Entry); err != nil {
			return nil, false, err
		}
	}
	return lib, usedDefault, nil
}
