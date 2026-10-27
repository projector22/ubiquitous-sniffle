package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projector22/ubiquitous-sniffle/internal/library"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func testLib(t *testing.T) *library.Library {
	t.Helper()
	l := library.New()
	add := func(artist, album string, e library.Entry) {
		if err := l.Add(artist, album, e); err != nil {
			t.Fatal(err)
		}
	}
	add("Daft Punk", "Discovery", library.Entry{Spotify: "https://open.spotify.com/album/a", YouTube: "https://music.youtube.com/playlist?list=b"})
	add("Daft Punk", "Homework", library.Entry{Spotify: "https://open.spotify.com/album/c"})
	add("ABBA", "Arrival", library.Entry{YouTube: "https://youtu.be/x"})
	add("AC/DC", "Back in Black", library.Entry{Spotify: "https://open.spotify.com/album/d"})
	add("宇多田ヒカル", "First Love", library.Entry{Spotify: "https://open.spotify.com/album/e"})
	return l
}

// model returns a sized model (4 artists: ABBA, AC/DC, Daft Punk, 宇多田ヒカル).
func model(t *testing.T, w, h int) *Model {
	t.Helper()
	m := NewModel(testLib(t))
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

var keyTypes = map[string]tea.KeyType{
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"enter": tea.KeyEnter, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "esc": tea.KeyEsc,
	"backspace": tea.KeyBackspace, "ctrl+c": tea.KeyCtrlC, "pgup": tea.KeyPgUp,
	"pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd, "space": tea.KeySpace,
}

// press sends keys by name ("down", "ctrl+c") or as typed text ("q", "/").
func press(m *Model, keys ...string) {
	for _, k := range keys {
		if t, ok := keyTypes[k]; ok {
			m.Update(tea.KeyMsg{Type: t})
			continue
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func click(m *Model, x, y int) {
	m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func wheel(m *Model, b tea.MouseButton, x, y int) {
	m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: b})
}

func screen(m *Model) string { return ansiRe.ReplaceAllString(m.View(), "") }

// --- navigation -----------------------------------------------------------

func TestInitialState(t *testing.T) {
	m := model(t, 100, 30)
	if m.SelectedArtist() != "ABBA" || m.SelectedAlbum() != "Arrival" {
		t.Errorf("selected %q / %q", m.SelectedArtist(), m.SelectedAlbum())
	}
	if m.Done() || m.Init() != nil {
		t.Error("should not start done and Init has no command")
	}
}

func TestKeyboardNavigation(t *testing.T) {
	m := model(t, 100, 30)
	press(m, "down", "down")
	if m.SelectedArtist() != "Daft Punk" {
		t.Fatalf("artist = %q", m.SelectedArtist())
	}
	press(m, "right", "j")
	if m.focus != paneAlbums || m.SelectedAlbum() != "Homework" {
		t.Errorf("focus=%v album=%q", m.focus, m.SelectedAlbum())
	}
	press(m, "k", "k") // clamps at the first album
	if m.SelectedAlbum() != "Discovery" {
		t.Errorf("album = %q", m.SelectedAlbum())
	}
	press(m, "left")
	if m.focus != paneArtists {
		t.Error("left should return to artists")
	}
	// Changing artist resets the album selection.
	press(m, "l", "down", "h", "j", "k")
	if m.SelectedArtist() != "Daft Punk" || m.SelectedAlbum() != "Discovery" {
		t.Errorf("selected %q / %q", m.SelectedArtist(), m.SelectedAlbum())
	}
	press(m, "tab")
	if m.focus != paneAlbums {
		t.Error("tab should move to albums")
	}
	press(m, "tab")
	if m.focus != paneArtists {
		t.Error("tab should toggle back")
	}
	press(m, "enter", "shift+tab")
	if m.focus != paneArtists {
		t.Error("shift+tab should return to artists")
	}
	press(m, "enter", "backspace")
	if m.focus != paneArtists {
		t.Error("backspace should return to artists")
	}
}

func TestJumpsAndPaging(t *testing.T) {
	m := model(t, 100, 30)
	for _, tc := range []struct{ key, want string }{
		{"end", "宇多田ヒカル"}, {"home", "ABBA"}, {"G", "宇多田ヒカル"}, {"g", "ABBA"},
		{"pgdown", "宇多田ヒカル"}, {"pgup", "ABBA"},
	} {
		press(m, tc.key)
		if m.SelectedArtist() != tc.want {
			t.Errorf("%s -> %q, want %q", tc.key, m.SelectedArtist(), tc.want)
		}
	}
}

func TestScrollingKeepsSelectionVisible(t *testing.T) {
	l := library.New()
	for i := 0; i < 50; i++ {
		_ = l.Add(string(rune('A'+i/26))+string(rune('a'+i%26)), "Album", library.Entry{Spotify: "s"})
	}
	m := NewModel(l)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	rows := m.innerRows()
	for i := 0; i < 30; i++ {
		press(m, "down")
		if m.artistSel < m.artistTop || m.artistSel >= m.artistTop+rows {
			t.Fatalf("selection %d outside window [%d,%d)", m.artistSel, m.artistTop, m.artistTop+rows)
		}
	}
	if m.artistTop == 0 {
		t.Error("list should have scrolled")
	}
	press(m, "home")
	if m.artistTop != 0 || m.artistSel != 0 {
		t.Errorf("Home: sel=%d top=%d", m.artistSel, m.artistTop)
	}
	press(m, "end")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	if m.artistSel < m.artistTop || m.artistSel >= m.artistTop+m.innerRows() {
		t.Errorf("after resize sel=%d top=%d rows=%d", m.artistSel, m.artistTop, m.innerRows())
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c", "esc"} {
		m := model(t, 100, 30)
		_, cmd := m.Update(tea.KeyMsg{Type: keyTypeOr(k), Runes: runesOr(k)})
		if !m.Done() || m.Result().Action != ActionNone {
			t.Errorf("%s should quit without an action", k)
		}
		if cmd == nil {
			t.Errorf("%s should return tea.Quit", k)
		} else if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: command did not produce tea.QuitMsg", k)
		}
	}
}

func keyTypeOr(k string) tea.KeyType {
	if t, ok := keyTypes[k]; ok {
		return t
	}
	return tea.KeyRunes
}

func runesOr(k string) []rune {
	if _, ok := keyTypes[k]; ok {
		return nil
	}
	return []rune(k)
}

func TestDownloadSelection(t *testing.T) {
	m := model(t, 100, 30)
	press(m, "down", "down", "d") // artist pane -> whole artist
	if r := m.Result(); !m.Done() || r.Action != ActionDownload || r.Artist != "Daft Punk" || r.Album != "" {
		t.Errorf("artist download: %+v", r)
	}

	m = model(t, 100, 30)
	press(m, "down", "down", "l", "j", "d")
	if r := m.Result(); r.Artist != "Daft Punk" || r.Album != "Homework" || r.Action != ActionDownload {
		t.Errorf("album download: %+v", r)
	}

	m = NewModel(library.New())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(m, "d")
	if m.Done() || !strings.Contains(screen(m), "nothing selected") {
		t.Error("download on an empty library should show a status, not exit")
	}
}

func TestFilter(t *testing.T) {
	m := model(t, 100, 30)
	press(m, "/")
	typeText(m, "daf")
	if !m.filtering || len(m.artists) != 1 || m.SelectedArtist() != "Daft Punk" {
		t.Fatalf("filter 'daf': artists=%v", m.artists)
	}
	// Matches album names too.
	press(m, "backspace", "backspace", "backspace")
	typeText(m, "first")
	if len(m.artists) != 1 || m.SelectedArtist() != "宇多田ヒカル" {
		t.Errorf("album-name filter: %v", m.artists)
	}
	// Enter keeps the filter, esc (normal mode) clears it, esc again quits.
	press(m, "enter")
	if m.filtering || m.filter != "first" {
		t.Errorf("enter: filtering=%v filter=%q", m.filtering, m.filter)
	}
	press(m, "esc")
	if m.filter != "" || len(m.artists) != 4 || m.Done() {
		t.Errorf("esc should clear the filter: %q %d done=%v", m.filter, len(m.artists), m.Done())
	}
	press(m, "esc")
	if !m.Done() {
		t.Error("second esc should quit")
	}
}

func TestFilterEdgeCases(t *testing.T) {
	m := model(t, 100, 30)
	press(m, "/", "esc") // esc inside filter mode clears and leaves it
	if m.filtering || m.Done() {
		t.Error("esc in filter mode should only leave it")
	}
	press(m, "/", "backspace") // backspace on empty leaves filter mode
	if m.filtering {
		t.Error("backspace on an empty filter should leave filter mode")
	}
	press(m, "/")
	typeText(m, "zzz")
	if len(m.artists) != 0 || m.SelectedArtist() != "" {
		t.Errorf("no match expected: %v", m.artists)
	}
	if !strings.Contains(screen(m), "No artists match") {
		t.Errorf("missing no-match message:\n%s", screen(m))
	}
	press(m, "down", "up", "enter", "l") // harmless on an empty list
	if m.focus != paneArtists {
		t.Error("cannot focus albums when nothing matches")
	}

	// Space is part of the filter; arrows navigate while filtering; the
	// selection survives re-filtering while it still matches.
	m = model(t, 100, 30)
	press(m, "/", "down", "down")
	sel := m.SelectedArtist() // Daft Punk
	press(m, "space")
	typeText(m, "p")
	if m.filter != " p" || m.SelectedArtist() != sel {
		t.Errorf("filter=%q selected=%q (was %q)", m.filter, m.SelectedArtist(), sel)
	}
	press(m, "up")
}

// --- mouse ----------------------------------------------------------------

func TestMouse(t *testing.T) {
	m := model(t, 100, 30)
	ar, br, dr := m.layout()
	first := func(r rect) int { return r.y + 2 } // below the border and title line

	click(m, ar.x+3, first(ar)+2) // third artist
	if m.SelectedArtist() != "Daft Punk" || m.focus != paneArtists {
		t.Fatalf("click artist: %q focus=%v", m.SelectedArtist(), m.focus)
	}
	click(m, br.x+3, first(br)+1) // second album
	if m.SelectedAlbum() != "Homework" || m.focus != paneAlbums {
		t.Errorf("click album: %q focus=%v", m.SelectedAlbum(), m.focus)
	}
	click(m, br.x+3, first(br)+9) // empty row: selection unchanged
	if m.SelectedAlbum() != "Homework" {
		t.Errorf("empty-row click changed the selection: %q", m.SelectedAlbum())
	}
	click(m, ar.x+3, ar.y+1) // title line
	click(m, dr.x+3, dr.y+2) // details pane
	m.Update(tea.MouseMsg{X: ar.x + 3, Y: first(ar), Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m.SelectedArtist() != "Daft Punk" {
		t.Errorf("artist changed unexpectedly: %q", m.SelectedArtist())
	}

	wheel(m, tea.MouseButtonWheelUp, ar.x+3, ar.y+3)
	if m.SelectedArtist() != "AC/DC" {
		t.Errorf("wheel up on artists: %q", m.SelectedArtist())
	}
	wheel(m, tea.MouseButtonWheelDown, ar.x+3, ar.y+3)
	if m.SelectedArtist() != "Daft Punk" {
		t.Errorf("wheel down on artists: %q", m.SelectedArtist())
	}
	wheel(m, tea.MouseButtonWheelDown, br.x+3, br.y+3)
	if m.focus != paneAlbums || m.SelectedAlbum() != "Homework" {
		t.Errorf("wheel on albums: focus=%v album=%q", m.focus, m.SelectedAlbum())
	}
	wheel(m, tea.MouseButtonWheelDown, dr.x+3, dr.y+3) // over details: ignored
	m.Update(tea.MouseMsg{X: 1, Y: 1, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
}

// The mouse maths assumes list rows start two lines below a pane's top edge.
// Verify against the real rendering so layout changes cannot silently break it.
func TestMouseRowsMatchRendering(t *testing.T) {
	m := model(t, 100, 30)
	ar, br, _ := m.layout()
	lines := strings.Split(screen(m), "\n")
	if !strings.Contains(lines[ar.y+2][ar.x:ar.x+ar.w], "▸ ABBA") {
		t.Errorf("first artist row not at y=%d: %q", ar.y+2, lines[ar.y+2])
	}
	if !strings.Contains(lines[br.y+2], "Arrival") {
		t.Errorf("first album row not at y=%d: %q", br.y+2, lines[br.y+2])
	}
}

// --- rendering ------------------------------------------------------------

func assertFrame(t *testing.T, m *Model, w, h int) {
	t.Helper()
	lines := strings.Split(screen(m), "\n")
	if len(lines) != h {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), h, screen(m))
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got != w {
			t.Errorf("line %d has width %d, want %d: %q", i, got, w, l)
		}
	}
}

func TestViewLayout(t *testing.T) {
	m := model(t, 100, 20)
	assertFrame(t, m, 100, 20)
	view := screen(m)
	for _, want := range []string{"mdl", "4 artists", "5 albums", "Artists (4)", "Albums (1)", "Details", "▸ ABBA", "Arrival", "- Y", "d download", "press d to download every album", "ABBA/"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestViewSizes(t *testing.T) {
	for _, sz := range [][2]int{{60, 9}, {80, 24}, {101, 31}, {200, 50}} {
		assertFrame(t, model(t, sz[0], sz[1]), sz[0], sz[1])
	}
}

func TestViewAlbumDetails(t *testing.T) {
	m := model(t, 120, 24)
	press(m, "down", "down", "l")
	view := screen(m)
	for _, want := range []string{"Discovery", "by Daft Punk", "Spotify", "YouTube", "open.spotify.com/album/a", "Daft Punk/Discovery", "S Y", "S -", "press d to download this album"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	// The folder preview shows the sanitised path.
	press(m, "left", "up", "right")
	if v := screen(m); !strings.Contains(v, "AC_DC/Back in Black") {
		t.Errorf("sanitised folder missing:\n%s", v)
	}
}

func TestViewMissingLinkShowsNone(t *testing.T) {
	m := model(t, 120, 24)
	press(m, "l") // ABBA / Arrival: YouTube only
	if v := screen(m); !strings.Contains(v, "(none)") {
		t.Errorf("expected (none) for missing Spotify link:\n%s", v)
	}
}

func TestSelectionMarkerFollowsFocus(t *testing.T) {
	m := model(t, 100, 20)
	if strings.Count(screen(m), "▸") != 1 {
		t.Errorf("exactly one selected row expected:\n%s", screen(m))
	}
	press(m, "l")
	if !strings.Contains(screen(m), "▸ Arrival") {
		t.Errorf("album marker missing:\n%s", screen(m))
	}
}

func TestViewWideCharactersStayAligned(t *testing.T) {
	m := model(t, 100, 20)
	press(m, "end")
	assertFrame(t, m, 100, 20)
	if !strings.Contains(screen(m), "宇多田ヒカル") {
		t.Error("CJK name missing")
	}
}

func TestViewLongNamesAreTruncated(t *testing.T) {
	l := library.New()
	_ = l.Add(strings.Repeat("Very Long Artist ", 10), strings.Repeat("Long Album ", 10), library.Entry{Spotify: "https://open.spotify.com/album/" + strings.Repeat("x", 200)})
	m := NewModel(l)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	press(m, "l")
	assertFrame(t, m, 80, 20)
	if !strings.Contains(screen(m), "…") {
		t.Error("expected an ellipsis for truncated names")
	}
}

func TestViewSanitisesHostileNames(t *testing.T) {
	l := library.New()
	_ = l.Add("Evil\x1b[2J\nArtist", "Al\x07bum", library.Entry{Spotify: "https://open.spotify.com/a\x1b[31m"})
	m := NewModel(l)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	press(m, "l")
	if strings.ContainsAny(screen(m), "\x1b\x07") {
		t.Errorf("raw control character leaked into the view: %q", screen(m))
	}
	assertFrame(t, m, 100, 20)
}

func TestViewEmptyBeforeSizeAndTooSmall(t *testing.T) {
	m := NewModel(testLib(t))
	if m.View() != "" {
		t.Error("nothing to draw before the first WindowSizeMsg")
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})
	if v := screen(m); !strings.Contains(v, "Terminal too small") {
		t.Errorf("small terminal view: %q", v)
	}
	m = NewModel(library.New())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if v := screen(m); !strings.Contains(v, "library is empty") {
		t.Errorf("missing empty message:\n%s", v)
	}
}

func TestHeaderAndFooterStates(t *testing.T) {
	m := model(t, 100, 20)
	press(m, "/")
	view := screen(m)
	if !strings.Contains(view, "type to filter") || !strings.Contains(view, "/▏") {
		t.Errorf("filtering state missing:\n%s", view)
	}
	typeText(m, "x")
	press(m, "enter")
	if v := screen(m); !strings.Contains(v, "filter: x (esc clears)") {
		t.Errorf("active-filter header missing:\n%s", v)
	}
}

func TestHelpers(t *testing.T) {
	if plural(1) != "" || plural(2) != "s" {
		t.Error("plural")
	}
	if orNone("") != "(none)" || orNone("x") != "x" {
		t.Error("orNone")
	}
	if clamp(5, 0, 3) != 3 || clamp(-1, 0, 3) != 0 || clamp(1, 0, -1) != 0 {
		t.Error("clamp")
	}
	if badges(library.Entry{Spotify: "s"}) != "S -" || badges(library.Entry{YouTube: "y"}) != "- Y" {
		t.Error("badges")
	}
	if (rect{1, 1, 2, 2}).contains(3, 1) {
		t.Error("rect.contains")
	}
	if got := cleanText("a\x1b[31m\nb"); strings.ContainsAny(got, "\x1b\n") {
		t.Errorf("cleanText left control chars: %q", got)
	}
	if got := listLine("x", "", true, true, 10); lipgloss.Width(got) != 10 {
		t.Errorf("listLine width = %d", lipgloss.Width(got))
	}
	if visible([]int{1, 2}, 5, 3) != nil {
		t.Error("visible past the end should be nil")
	}
}

func TestUpdateIgnoresUnknownMessages(t *testing.T) {
	m := model(t, 100, 30)
	next, cmd := m.Update(struct{}{})
	if next != m || cmd != nil || m.Done() {
		t.Error("unknown messages must be ignored")
	}
}
