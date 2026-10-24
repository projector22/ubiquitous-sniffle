package library

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sample(t *testing.T) *Library {
	t.Helper()
	l := New()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(l.Add("Daft Punk", "Discovery", Entry{Spotify: "https://open.spotify.com/album/a?x=1&y=2", YouTube: "https://music.youtube.com/playlist?list=b"}))
	must(l.Add("daft punk", "Homework", Entry{Spotify: "https://open.spotify.com/album/c"}))
	must(l.Add("Zebra", "Stripes", Entry{YouTube: "https://youtu.be/xyz"}))
	must(l.Add("abba", "Arrival", Entry{Spotify: "https://open.spotify.com/album/d"}))
	return l
}

func TestClassifyURL(t *testing.T) {
	tests := []struct {
		in   string
		want Source
		ok   bool
	}{
		{"https://open.spotify.com/album/x", SourceSpotify, true},
		{"  https://spotify.com/x  ", SourceSpotify, true},
		{"https://music.youtube.com/playlist?list=x", SourceYouTube, true},
		{"https://www.youtube.com/watch?v=x", SourceYouTube, true},
		{"https://youtu.be/x", SourceYouTube, true},
		{"https://notspotify.com/x", SourceUnknown, false},
		{"https://evil.com/open.spotify.com", SourceUnknown, false},
		{"ftp://open.spotify.com/x", SourceUnknown, false},
		{"not a url", SourceUnknown, false},
		{"", SourceUnknown, false},
	}
	for _, tc := range tests {
		got, err := ClassifyURL(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("ClassifyURL(%q) = %v, %v; want %v ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func TestSourceString(t *testing.T) {
	if SourceSpotify.String() != "spotify" || SourceYouTube.String() != "youtube" || SourceUnknown.String() != "unknown" {
		t.Error("unexpected Source.String values")
	}
}

func TestEntrySetURL(t *testing.T) {
	var e Entry
	if !e.Empty() {
		t.Error("zero Entry should be empty")
	}
	if err := e.SetURL(" https://open.spotify.com/album/a "); err != nil {
		t.Fatal(err)
	}
	if err := e.SetURL("https://youtu.be/b"); err != nil {
		t.Fatal(err)
	}
	if e.Spotify != "https://open.spotify.com/album/a" || e.YouTube != "https://youtu.be/b" || e.Empty() {
		t.Errorf("unexpected entry %+v", e)
	}
	if err := e.SetURL("https://example.com"); err == nil {
		t.Error("expected error for unsupported host")
	}
}

func TestAddValidationAndMerge(t *testing.T) {
	l := New()
	good := Entry{Spotify: "s"}
	for name, fn := range map[string]func() error{
		"no artist": func() error { return l.Add(" ", "a", good) },
		"no album":  func() error { return l.Add("a", "", good) },
		"no links":  func() error { return l.Add("a", "b", Entry{}) },
	} {
		if fn() == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	if err := l.Add("Artist", "Album", Entry{Spotify: "s1"}); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive match; adding YouTube keeps Spotify, replacing Spotify overwrites.
	if err := l.Add("ARTIST", "album", Entry{YouTube: "y1", Spotify: "s2"}); err != nil {
		t.Fatal(err)
	}
	if l.NumArtists() != 1 || l.NumAlbums() != 1 {
		t.Fatalf("expected 1 artist / 1 album, got %d / %d", l.NumArtists(), l.NumAlbums())
	}
	e, ok := l.Get("Artist", "Album")
	if !ok || e.Spotify != "s2" || e.YouTube != "y1" {
		t.Errorf("unexpected entry %+v ok=%v", e, ok)
	}
	if err := l.Add("Artist", "Album", Entry{YouTube: "y2"}); err != nil {
		t.Fatal(err)
	}
	if e, _ := l.Get("Artist", "Album"); e.Spotify != "s2" || e.YouTube != "y2" {
		t.Errorf("empty field should be preserved: %+v", e)
	}
	if _, ok := l.Get("Nope", "Album"); ok {
		t.Error("Get on missing artist should be false")
	}
}

func TestSortingAndResolve(t *testing.T) {
	l := sample(t)
	if got, want := l.Artists(), []string{"abba", "Daft Punk", "Zebra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Artists = %v, want %v", got, want)
	}
	if got, want := l.Albums("Daft Punk"), []string{"Discovery", "Homework"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Albums = %v, want %v", got, want)
	}
	if a, ok := l.ResolveArtist("  ZEBRA "); !ok || a != "Zebra" {
		t.Errorf("ResolveArtist = %q, %v", a, ok)
	}
	if _, ok := l.ResolveArtist("nobody"); ok {
		t.Error("expected miss")
	}
	if a, ok := l.ResolveAlbum("Daft Punk", "HOMEWORK"); !ok || a != "Homework" {
		t.Errorf("ResolveAlbum = %q, %v", a, ok)
	}
	if _, ok := l.ResolveAlbum("Daft Punk", "nope"); ok {
		t.Error("expected miss")
	}
	if l.NumArtists() != 3 || l.NumAlbums() != 4 {
		t.Errorf("counts = %d/%d", l.NumArtists(), l.NumAlbums())
	}
}

func TestSortedKeysTieBreak(t *testing.T) {
	got := sortedKeys(map[string]int{"b": 1, "B": 1, "a": 1})
	if want := []string{"a", "B", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	l := sample(t)
	path := filepath.Join(t.TempDir(), "nested", "dir", "library.json")
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "x=1&y=2") || strings.Contains(string(raw), `\u0026`) {
		t.Errorf("'&' must not be HTML-escaped:\n%s", raw)
	}
	if !strings.Contains(string(raw), "\n  \"Daft Punk\": {\n    \"Discovery\": {\n") {
		t.Errorf("expected two-space indentation:\n%s", raw)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.data, l.data) {
		t.Errorf("round trip differs:\n%v\n%v", back.data, l.data)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("expected only the library file, got %v", entries)
	}
}

func TestSaveErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New().Save(filepath.Join(blocker, "lib.json")); err == nil {
		t.Error("expected error when parent is a file")
	}
}

func TestLoadMissingAndLoadOrNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none.json")
	if _, err := Load(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
	l, err := LoadOrNew(path)
	if err != nil || l.NumAlbums() != 0 {
		t.Errorf("LoadOrNew = %v, %v", l, err)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte("{"), 0o644)
	if _, err := LoadOrNew(bad); err == nil {
		t.Error("LoadOrNew must surface parse errors")
	}
}

func TestReadVariants(t *testing.T) {
	tests := []struct {
		name, in string
		wantErr  string
		albums   int
	}{
		{"empty file", "", "", 0},
		{"whitespace", " \n ", "", 0},
		{"null", "null", "", 0},
		{"empty object", "{}", "", 0},
		{"artist without albums", `{"A": null}`, "", 0},
		{"valid", `{"A":{"B":{"spotify":"s"}}}`, "", 1},
		{"typo field", `{"A":{"B":{"youtbe":"s"}}}`, "unknown field", 0},
		{"legacy format", `{"A":{"B":"https://x"}}`, "mdl import", 0},
		{"trailing data", `{} {}`, "unexpected data", 0},
		{"empty artist", `{" ":{"B":{"spotify":"s"}}}`, "empty artist", 0},
		{"empty album", `{"A":{"":{"spotify":"s"}}}`, "empty album", 0},
		{"not json", `hello`, "invalid library", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := Read(strings.NewReader(tc.in))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if l.NumAlbums() != tc.albums {
				t.Errorf("albums = %d, want %d", l.NumAlbums(), tc.albums)
			}
		})
	}
}

func TestWriteEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := New().Write(&b); err != nil || b.String() != "{}\n" {
		t.Errorf("got %q, %v", b.String(), err)
	}
}

func TestMerge(t *testing.T) {
	a, b := sample(t), New()
	_ = b.Add("Zebra", "Stripes", Entry{Spotify: "https://open.spotify.com/album/z"})
	_ = b.Add("New", "Thing", Entry{Spotify: "s"})
	n, err := a.Merge(b)
	if err != nil || n != 2 {
		t.Fatalf("Merge = %d, %v", n, err)
	}
	if e, _ := a.Get("Zebra", "Stripes"); e.Spotify == "" || e.YouTube == "" {
		t.Errorf("merge should combine links: %+v", e)
	}
	if a.NumArtists() != 4 {
		t.Errorf("artists = %d", a.NumArtists())
	}
	// A bad entry inside other surfaces as an error.
	bad := &Library{data: map[string]map[string]Entry{"X": {"Y": {}}}}
	if _, err := a.Merge(bad); err == nil {
		t.Error("expected error merging an empty entry")
	}
}

func TestDefaultPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		env  map[string]string
		home string
		want string
	}{
		{map[string]string{"MDL_LIBRARY": "/x/lib.json", "XDG_CONFIG_HOME": "/cfg"}, "/home/u", "/x/lib.json"},
		{map[string]string{"XDG_CONFIG_HOME": "/cfg"}, "/home/u", filepath.Join("/cfg", "mdl", "library.json")},
		{nil, "/home/u", filepath.Join("/home/u", ".config", "mdl", "library.json")},
		{nil, "", "library.json"},
	}
	for _, tc := range tests {
		if got := DefaultPath(env(tc.env), tc.home); got != tc.want {
			t.Errorf("DefaultPath = %q, want %q", got, tc.want)
		}
	}
}

func TestImportBatch(t *testing.T) {
	t.Run("single artist", func(t *testing.T) {
		l, _, err := ImportBatch([]byte(`{"Discovery":"https://open.spotify.com/album/a","Homework":"https://music.youtube.com/playlist?list=b"}`), "Daft Punk")
		if err != nil {
			t.Fatal(err)
		}
		if e, _ := l.Get("Daft Punk", "Homework"); e.YouTube == "" {
			t.Errorf("Homework should be a YouTube link: %+v", e)
		}
		if l.NumAlbums() != 2 {
			t.Errorf("albums = %d", l.NumAlbums())
		}
	})
	t.Run("multi artist", func(t *testing.T) {
		l, _, err := ImportBatch([]byte(`{"A":{"X":"https://open.spotify.com/album/1"},"B":{"Y":"https://youtu.be/2"}}`), "")
		if err != nil || l.NumArtists() != 2 {
			t.Fatalf("got %v, %v", l, err)
		}
	})
	for name, tc := range map[string]struct{ in, artist, want string }{
		"single without artist": {`{"X":"https://open.spotify.com/a"}`, "", "--artist"},
		"bad url single":        {`{"X":"https://example.com"}`, "A", "neither"},
		"bad url multi":         {`{"A":{"X":"nope"}}`, "", "valid http"},
		"new format":            {`{"A":{"X":{"spotify":"s"}}}`, "", "already in the new format"},
		"number value":          {`{"A":5}`, "", "unsupported"},
		"not an object":         {`[1]`, "", "not a JSON object"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := ImportBatch([]byte(tc.in), tc.artist)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestBatchTemplateAndPlaceholders(t *testing.T) {
	tmpl := BatchTemplate()
	if !strings.Contains(string(tmpl), SampleAlbum) || !strings.Contains(string(tmpl), SampleURL) {
		t.Errorf("template = %s", tmpl)
	}
	// The untouched template imports nothing, whichever artist is supplied.
	l, used, err := ImportBatch(tmpl, "Anyone")
	if err != nil || l.NumAlbums() != 0 || used {
		t.Errorf("template import: %d albums, used=%v, err=%v", l.NumAlbums(), used, err)
	}
	// ...even without a default artist (a pure placeholder needs none).
	if _, _, err := ImportBatch(tmpl, ""); err != nil {
		t.Errorf("template without artist: %v", err)
	}
	// Placeholders inside a nested file are skipped too, real albums kept.
	l, used, err = ImportBatch([]byte(`{"A": {"X": "`+SampleURL+`", "Y": "https://youtu.be/y"}}`), "")
	if err != nil || l.NumAlbums() != 1 || used {
		t.Errorf("nested: %d albums, used=%v, err=%v", l.NumAlbums(), used, err)
	}
}

func TestImportBatchReportsWhetherTheDefaultArtistWasUsed(t *testing.T) {
	_, used, err := ImportBatch([]byte(`{"Album": "https://youtu.be/y"}`), "Given")
	if err != nil || !used {
		t.Errorf("flat: used=%v err=%v", used, err)
	}
	_, used, err = ImportBatch([]byte(`{"Artist": {"Album": "https://youtu.be/y"}}`), "Given")
	if err != nil || used {
		t.Errorf("nested: used=%v err=%v", used, err)
	}
}
