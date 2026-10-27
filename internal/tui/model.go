// Package tui is the interactive library browser, built on Bubble Tea
// (github.com/charmbracelet/bubbletea) with Lip Gloss styling.
//
// Three panes - artists, albums, details - are driven by keyboard or mouse.
// Model is a plain tea.Model whose only I/O is the save callback it is given
// (used after a delete), so it is unit tested by feeding it messages and
// inspecting View(); Run (tui.go) starts the program.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/projector22/ubiquitous-sniffle/internal/downloader"
	"github.com/projector22/ubiquitous-sniffle/internal/library"
)

// Action is what the caller should do after the TUI exits.
type Action int

// Possible actions.
const (
	ActionNone Action = iota
	ActionDownload
)

// Result is returned by Run. For ActionDownload, Album is empty when the
// whole artist was selected.
type Result struct {
	Action        Action
	Artist, Album string
}

type pane int

const (
	paneArtists pane = iota
	paneAlbums
)

const (
	minW = 60
	minH = 9

	// chrome: one header line, one footer line, and inside each pane a
	// border (2 lines) plus a title line.
	paneChrome = 3
)

// Colours (ANSI 256 palette, degraded automatically on poor terminals).
var (
	colTitle  = lipgloss.Color("213")
	colFocus  = lipgloss.Color("75")
	colBorder = lipgloss.Color("240")
	colDim    = lipgloss.Color("245")
	colAccent = lipgloss.Color("114")
	colSelBg  = lipgloss.Color("63")
	colBlurBg = lipgloss.Color("238")
	colText   = lipgloss.Color("255")

	styTitle   = lipgloss.NewStyle().Bold(true).Foreground(colTitle)
	styHeading = lipgloss.NewStyle().Bold(true) // default foreground: readable on light and dark themes
	styDim     = lipgloss.NewStyle().Foreground(colDim)
	styAccent  = lipgloss.NewStyle().Foreground(colAccent)
	selFocus   = lipgloss.NewStyle().Bold(true).Foreground(colText).Background(colSelBg)
	selBlur    = lipgloss.NewStyle().Foreground(colText).Background(colBlurBg)
)

// Model is the TUI state. It implements tea.Model.
type Model struct {
	lib *library.Library
	w   int
	h   int

	all     []string // every artist
	artists []string // artists passing the filter
	albums  []string // albums of the selected artist

	artistSel, artistTop int
	albumSel, albumTop   int
	focus                pane

	filter    string
	filtering bool
	status    string

	// A delete waiting for y/n. delAlbum is empty for a whole artist.
	confirming          bool
	delArtist, delAlbum string
	save                func() error

	done   bool
	result Result
}

var _ tea.Model = (*Model)(nil)

// NewModel builds a model for lib. save is called after every delete to
// persist lib; nil keeps changes in memory only. The size arrives with the
// first tea.WindowSizeMsg.
func NewModel(lib *library.Library, save func() error) *Model {
	m := &Model{lib: lib, all: lib.Artists(), save: save}
	m.applyFilter()
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Done reports whether the user finished (quit or chose an action).
func (m *Model) Done() bool { return m.done }

// Result returns the chosen action; valid once Done is true.
func (m *Model) Result() Result { return m.result }

// SelectedArtist returns the highlighted artist, or "".
func (m *Model) SelectedArtist() string {
	if m.artistSel < len(m.artists) {
		return m.artists[m.artistSel]
	}
	return ""
}

// SelectedAlbum returns the highlighted album, or "".
func (m *Model) SelectedAlbum() string {
	if m.albumSel < len(m.albums) {
		return m.albums[m.albumSel]
	}
	return ""
}

// innerRows is how many list rows fit in a pane.
func (m *Model) innerRows() int {
	if r := m.h - 2 - paneChrome; r > 1 {
		return r
	}
	return 1
}

func (m *Model) resize(w, h int) {
	m.w, m.h = w, h
	m.artistTop = ensureVisible(m.artistSel, m.artistTop, m.innerRows())
	m.albumTop = ensureVisible(m.albumSel, m.albumTop, m.innerRows())
}

func ensureVisible(sel, top, rows int) int {
	switch {
	case sel < top:
		return sel
	case sel >= top+rows:
		return sel - rows + 1
	}
	return top
}

func clamp(v, lo, hi int) int {
	if hi < lo || v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// applyFilter recomputes the artist list from m.filter, keeping the current
// artist selected if it survives. An artist matches when its name or any of
// its album names contains the filter (case-insensitive).
func (m *Model) applyFilter() {
	prev := m.SelectedArtist()
	q := strings.ToLower(m.filter)
	var out []string
	for _, a := range m.all {
		if q == "" || m.matches(a, q) {
			out = append(out, a)
		}
	}
	m.artists = out
	m.artistSel = 0
	for i, a := range out {
		if a == prev {
			m.artistSel = i
		}
	}
	m.artistTop = ensureVisible(m.artistSel, 0, m.innerRows())
	m.loadAlbums()
	if len(m.artists) == 0 {
		m.focus = paneArtists
	}
}

func (m *Model) matches(artist, q string) bool {
	if strings.Contains(strings.ToLower(artist), q) {
		return true
	}
	for _, al := range m.lib.Albums(artist) {
		if strings.Contains(strings.ToLower(al), q) {
			return true
		}
	}
	return false
}

func (m *Model) loadAlbums() {
	m.albums = nil
	if a := m.SelectedArtist(); a != "" {
		m.albums = m.lib.Albums(a)
	}
	m.albumSel, m.albumTop = 0, 0
}

func (m *Model) setArtist(i int) {
	i = clamp(i, 0, len(m.artists)-1)
	if i != m.artistSel {
		m.artistSel = i
		m.loadAlbums()
	}
	m.artistTop = ensureVisible(m.artistSel, m.artistTop, m.innerRows())
}

func (m *Model) setAlbum(i int) {
	m.albumSel = clamp(i, 0, len(m.albums)-1)
	m.albumTop = ensureVisible(m.albumSel, m.albumTop, m.innerRows())
}

func (m *Model) move(delta int) {
	if m.focus == paneArtists {
		m.setArtist(m.artistSel + delta)
	} else {
		m.setAlbum(m.albumSel + delta)
	}
}

func (m *Model) focusAlbums() {
	if len(m.albums) > 0 {
		m.focus = paneAlbums
	}
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.MouseMsg:
		m.mouse(msg)
	case tea.KeyMsg:
		m.status = ""
		switch {
		case msg.String() == "ctrl+c":
			m.done = true
		case m.confirming:
			m.confirmKey(msg)
		case m.filtering:
			m.filterKey(msg)
		default:
			m.normalKey(msg)
		}
	}
	if m.done {
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) normalKey(msg tea.KeyMsg) {
	page := m.innerRows()
	switch msg.String() {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-page)
	case "pgdown":
		m.move(page)
	case "home", "g":
		m.move(-1 << 30)
	case "end", "G":
		m.move(1 << 30)
	case "right", "l", "enter":
		m.focusAlbums()
	case "tab":
		if m.focus == paneAlbums {
			m.focus = paneArtists
		} else {
			m.focusAlbums()
		}
	case "left", "h", "shift+tab", "backspace":
		m.focus = paneArtists
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
		} else {
			m.done = true
		}
	case "q":
		m.done = true
	case "/":
		m.filtering = true
	case "d":
		m.requestDownload()
	case "x", "delete":
		m.requestDelete()
	}
}

func (m *Model) filterKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filter, m.filtering = "", false
		m.applyFilter()
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if m.filter == "" {
			m.filtering = false
			return
		}
		rs := []rune(m.filter)
		m.filter = string(rs[:len(rs)-1])
		m.applyFilter()
	case tea.KeyUp:
		m.move(-1)
	case tea.KeyDown:
		m.move(1)
	case tea.KeySpace:
		m.filter += " "
		m.applyFilter()
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.applyFilter()
	}
}

func (m *Model) requestDownload() {
	artist := m.SelectedArtist()
	if artist == "" {
		m.status = "nothing selected"
		return
	}
	r := Result{Action: ActionDownload, Artist: artist}
	if m.focus == paneAlbums {
		r.Album = m.SelectedAlbum()
	}
	m.result, m.done = r, true
}

// requestDelete asks to confirm deleting the selected album (albums pane) or
// artist (artists pane).
func (m *Model) requestDelete() {
	artist := m.SelectedArtist()
	if artist == "" {
		m.status = "nothing selected"
		return
	}
	m.delArtist, m.delAlbum = artist, ""
	if m.focus == paneAlbums {
		m.delAlbum = m.SelectedAlbum()
	}
	m.confirming = true
}

func (m *Model) confirmKey(msg tea.KeyMsg) {
	m.confirming = false
	if k := msg.String(); k != "y" && k != "Y" {
		m.status = "nothing deleted"
		return
	}
	m.deleteSelection()
}

// deleteSelection removes the confirmed artist or album and saves. If saving
// fails the library is put back as it was so memory matches the file.
func (m *Model) deleteSelection() {
	backup := m.lib.Clone()
	what := cleanText(m.delArtist)
	if m.delAlbum == "" {
		m.lib.RemoveArtist(m.delArtist)
	} else {
		m.lib.Remove(m.delArtist, m.delAlbum)
		what = cleanText(m.delAlbum) + " by " + what
	}
	if m.save != nil {
		if err := m.save(); err != nil {
			*m.lib = *backup
			m.status = "delete failed: " + cleanText(err.Error())
			return
		}
	}
	m.refresh()
	m.status = "deleted " + what
}

// refresh reloads the lists after the library changed, keeping the selection
// at the same positions where possible.
func (m *Model) refresh() {
	artistSel, albumSel, focus := m.artistSel, m.albumSel, m.focus
	m.all = m.lib.Artists()
	m.applyFilter()
	if m.SelectedArtist() != m.delArtist {
		// The artist is gone: select whichever one moved into its place.
		m.artistSel = clamp(artistSel, 0, len(m.artists)-1)
		m.artistTop = ensureVisible(m.artistSel, m.artistTop, m.innerRows())
		m.loadAlbums()
		m.focus = paneArtists
		return
	}
	m.setAlbum(albumSel)
	m.focus = focus
	if len(m.albums) == 0 {
		m.focus = paneArtists
	}
}

// --- layout & mouse -------------------------------------------------------

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// layout returns the outer rectangles (borders included) of the three panes.
func (m *Model) layout() (artists, albums, details rect) {
	aw := m.w * 28 / 100
	bw := m.w * 30 / 100
	h := m.h - 2
	artists = rect{0, 1, aw, h}
	albums = rect{aw, 1, bw, h}
	details = rect{aw + bw, 1, m.w - aw - bw, h}
	return
}

// rowAt maps a screen row inside pane r to a list index. The first list row
// sits below the top border and the title line.
func (m *Model) rowAt(r rect, top, n, y int) (int, bool) {
	row := y - (r.y + 2)
	if row < 0 || row >= m.innerRows() || top+row >= n {
		return 0, false
	}
	return top + row, true
}

func (m *Model) mouse(msg tea.MouseMsg) {
	if msg.Action != tea.MouseActionPress || m.confirming {
		return
	}
	ar, br, _ := m.layout()
	switch msg.Button {
	case tea.MouseButtonLeft:
		if ar.contains(msg.X, msg.Y) {
			m.focus = paneArtists
			if i, ok := m.rowAt(ar, m.artistTop, len(m.artists), msg.Y); ok {
				m.setArtist(i)
			}
		} else if br.contains(msg.X, msg.Y) {
			m.focusAlbums()
			if i, ok := m.rowAt(br, m.albumTop, len(m.albums), msg.Y); ok {
				m.setAlbum(i)
			}
		}
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		d := 1
		if msg.Button == tea.MouseButtonWheelUp {
			d = -1
		}
		if ar.contains(msg.X, msg.Y) {
			m.focus = paneArtists
			m.move(d)
		} else if br.contains(msg.X, msg.Y) {
			m.focusAlbums()
			m.move(d)
		}
	}
}

// --- rendering ------------------------------------------------------------

// cleanText replaces control characters (newlines, ESC, ...) with spaces so
// untrusted names in the library file can never inject terminal sequences.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
}

// listLine renders one selectable row exactly width cells wide: a marker, the
// (truncated) text, and an optional right-aligned note.
func listLine(text, right string, selected, focused bool, width int) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}
	leftW := width
	if right != "" {
		leftW = width - lipgloss.Width(right) - 1
	}
	left := ansi.Truncate(marker+cleanText(text), leftW, "…")
	left += strings.Repeat(" ", clamp(leftW-lipgloss.Width(left), 0, width))
	line := left
	if right != "" {
		line += " " + right
	}
	switch {
	case selected && focused:
		return selFocus.Render(line)
	case selected:
		return selBlur.Render(line)
	}
	return line
}

func (m *Model) renderPane(r rect, title string, lines []string, focused bool) string {
	border := colBorder
	if focused {
		border = colFocus
	}
	inner := r.w - 2
	body := styHeading.Render(ansi.Truncate(" "+title, inner, "…"))
	if len(lines) > 0 {
		body += "\n" + strings.Join(lines, "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(inner).
		Height(r.h - 2).
		Render(body)
}

func badges(e library.Entry) string {
	s, y, b := "-", "-", "-"
	if e.Spotify != "" {
		s = "S"
	}
	if e.YouTube != "" {
		y = "Y"
	}
	if e.Bandcamp != "" {
		b = "B"
	}
	return s + " " + y + " " + b
}

func visible[T any](items []T, top, n int) []T {
	if top >= len(items) {
		return nil
	}
	end := top + n
	if end > len(items) {
		end = len(items)
	}
	return items[top:end]
}

func (m *Model) artistLines(width int) []string {
	var lines []string
	for i, a := range visible(m.artists, m.artistTop, m.innerRows()) {
		lines = append(lines, listLine(a, fmt.Sprintf("%d", len(m.lib.Albums(a))),
			m.artistTop+i == m.artistSel, m.focus == paneArtists, width))
	}
	return lines
}

func (m *Model) albumLines(width int) []string {
	artist := m.SelectedArtist()
	var lines []string
	for i, al := range visible(m.albums, m.albumTop, m.innerRows()) {
		e, _ := m.lib.Get(artist, al)
		lines = append(lines, listLine(al, badges(e),
			m.albumTop+i == m.albumSel && m.focus == paneAlbums, m.focus == paneAlbums, width))
	}
	return lines
}

// detailLines builds the right-hand pane: album details when the albums pane
// has focus, an artist summary otherwise.
func (m *Model) detailLines(width int) []string {
	artist := m.SelectedArtist()
	var lines []string
	add := func(style lipgloss.Style, text string) {
		for _, l := range strings.Split(ansi.Wrap(cleanText(text), width-2, ""), "\n") {
			lines = append(lines, " "+style.Render(l))
		}
	}
	blank := func() { lines = append(lines, "") }

	if artist == "" {
		if m.filter != "" {
			add(styDim, "No artists match the filter.")
		} else {
			add(styDim, "The library is empty. Add albums with `mdl add`.")
		}
		return lines
	}

	plain := lipgloss.NewStyle()
	if m.focus == paneAlbums && m.SelectedAlbum() != "" {
		album := m.SelectedAlbum()
		e, _ := m.lib.Get(artist, album)
		add(styHeading, album)
		add(styDim, "by "+artist)
		blank()
		add(styAccent, "Spotify")
		add(plain, orNone(e.Spotify))
		blank()
		add(styAccent, "YouTube")
		add(plain, orNone(e.YouTube))
		blank()
		add(styAccent, "Bandcamp")
		add(plain, orNone(e.Bandcamp))
		blank()
		add(styAccent, "Folder")
		add(plain, downloader.SanitizeName(artist)+"/"+downloader.SanitizeName(album))
		blank()
		add(styDim, "press d to download this album, x to delete it")
		return lines
	}

	n := len(m.albums)
	sp, yt, bc := 0, 0, 0
	for _, al := range m.albums {
		e, _ := m.lib.Get(artist, al)
		if e.Spotify != "" {
			sp++
		}
		if e.YouTube != "" {
			yt++
		}
		if e.Bandcamp != "" {
			bc++
		}
	}
	add(styHeading, artist)
	add(styDim, fmt.Sprintf("%d album%s", n, plural(n)))
	blank()
	add(styAccent, "Links")
	add(plain, fmt.Sprintf("%d Spotify, %d YouTube, %d Bandcamp", sp, yt, bc))
	blank()
	add(styAccent, "Folder")
	add(plain, downloader.SanitizeName(artist)+"/")
	blank()
	add(styDim, "press d to download every album, x to delete the artist")
	return lines
}

// fit cuts lines to the number of rows a pane can show, marking the cut, so
// long content can never push the frame past the terminal height.
func (m *Model) fit(lines []string) []string {
	rows := m.innerRows()
	if len(lines) <= rows {
		return lines
	}
	out := append([]string(nil), lines[:rows]...)
	out[rows-1] = " " + styDim.Render("…")
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// View implements tea.Model. It always returns exactly m.h lines of m.w
// cells so Bubble Tea never has to scroll or clip the frame.
func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		msg := fmt.Sprintf("Terminal too small (need %dx%d, have %dx%d). Press q to quit.", minW, minH, m.w, m.h)
		return lipgloss.NewStyle().Width(m.w).Height(m.h).MaxWidth(m.w).MaxHeight(m.h).Render(msg)
	}

	ar, br, dr := m.layout()
	artists := m.renderPane(ar, fmt.Sprintf("Artists (%d)", len(m.artists)), m.artistLines(ar.w-2), m.focus == paneArtists)
	albums := m.renderPane(br, fmt.Sprintf("Albums (%d)", len(m.albums)), m.albumLines(br.w-2), m.focus == paneAlbums)
	details := m.renderPane(dr, "Details", m.fit(m.detailLines(dr.w-2)), false)

	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		lipgloss.JoinHorizontal(lipgloss.Top, artists, albums, details),
		m.footer(),
	)
}

func (m *Model) header() string {
	title := " mdl"
	counts := fmt.Sprintf("  %d artists · %d albums", m.lib.NumArtists(), m.lib.NumAlbums())
	right := ""
	switch {
	case m.filtering:
		right = "/" + cleanText(m.filter) + "▏ "
	case m.filter != "":
		right = "filter: " + cleanText(m.filter) + " (esc clears) "
	}
	gap := clamp(m.w-lipgloss.Width(title)-lipgloss.Width(counts)-lipgloss.Width(right), 0, m.w)
	return styTitle.Render(title) + styDim.Render(counts) + strings.Repeat(" ", gap) + styAccent.Render(right)
}

func (m *Model) footer() string {
	text := " ↑↓ move   ←→ pane   click select   / filter   d download   x delete   q quit"
	switch {
	case m.confirming:
		what := "album " + cleanText(m.delAlbum) + " by " + cleanText(m.delArtist)
		if m.delAlbum == "" {
			n := len(m.lib.Albums(m.delArtist))
			what = fmt.Sprintf("artist %s and its %d album%s", cleanText(m.delArtist), n, plural(n))
		}
		return styAccent.Width(m.w).Render(ansi.Truncate(" delete "+what+"? y/n", m.w, "…"))
	case m.status != "":
		text = " " + m.status
	case m.filtering:
		text = " type to filter · enter keeps it · esc clears · ↑↓ move"
	}
	return styDim.Width(m.w).Render(ansi.Truncate(text, m.w, "…"))
}
