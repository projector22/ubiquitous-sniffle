package cli

import (
	"fmt"
	"strings"
)

const helpWidth = 78

// wrapWords wraps text to width columns on word boundaries.
func wrapWords(text string, width int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(text) {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// formatOpts renders options as an aligned two-column table:
//
//	-a, --artist NAME   Description...
func formatOpts(opts []Opt) string {
	const indent, gap = 2, 3
	labels := make([]string, len(opts))
	maxLen := 0
	for i, o := range opts {
		l := fmt.Sprintf("-%c, --%s", o.Short, o.Long)
		if o.Arg != "" {
			l += " " + o.Arg
		}
		labels[i] = l
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	col := indent + maxLen + gap
	var b strings.Builder
	for i, o := range opts {
		desc := o.Desc
		for alias, canon := range longAliases {
			if canon == o.Long {
				desc += " Also accepted as --" + alias + "."
			}
		}
		lines := wrapWords(desc, helpWidth-col)
		if len(lines) == 0 {
			lines = []string{""}
		}
		fmt.Fprintf(&b, "%s%-*s%s\n", strings.Repeat(" ", indent), maxLen+gap, labels[i], lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(&b, "%s%s\n", strings.Repeat(" ", col), l)
		}
	}
	return b.String()
}

func paragraph(text string) string {
	return "  " + strings.Join(wrapWords(text, helpWidth-2), "\n  ") + "\n"
}

// topHelp is the full `mdl --help` text.
func (a *App) topHelp() string {
	var b strings.Builder
	b.WriteString("mdl - download, organise and browse a personal music library\n\n")
	b.WriteString("USAGE\n  mdl                                           download ./mdl.json, if there is one\n" +
		"  mdl [OPTIONS] URL...                          download URLs into the current directory\n" +
		"  mdl [GLOBAL OPTIONS] <command> [OPTIONS] [ARGUMENTS]\n  mdl <command> --help\n\n")
	b.WriteString("DESCRIPTION\n")
	b.WriteString(paragraph("mdl keeps one library file that maps artist -> album -> Spotify / YouTube link, " +
		"downloads those albums with spotdl or yt-dlp into <music-dir>/<Artist>/<Album>/, " +
		"and can hand the results to beets (beet) for tagging. Every option has a long form " +
		"(--like-this) and a short form (-l); short flags can be combined (-sb) and values " +
		"can be attached (--artist=NAME or -aNAME)."))
	b.WriteString("\nCOMMANDS\n")
	cmds := a.commands()
	w := 0
	for _, c := range cmds {
		if len(c.usageName()) > w {
			w = len(c.usageName())
		}
	}
	for _, c := range cmds {
		fmt.Fprintf(&b, "  %-*s  %s\n", w, c.usageName(), c.Summary)
	}
	b.WriteString("\nGLOBAL OPTIONS (accepted before or after the command)\n")
	b.WriteString(formatOpts(globalOpts))
	b.WriteString("\nLIBRARY FORMAT\n")
	b.WriteString(paragraph("The library is a single JSON file. Keys are sorted and indented so it is easy to read, " +
		"diff and edit by hand (`mdl edit`). Each album needs at least one of \"spotify\" or \"youtube\":"))
	b.WriteString(`
    {
      "Daft Punk": {
        "Discovery": {
          "spotify": "https://open.spotify.com/album/2noRn2Aes5aoNVsU6iWThc",
          "youtube": "https://music.youtube.com/playlist?list=OLAK5uy_..."
        }
      }
    }
`)
	b.WriteString("\nFILES AND ENVIRONMENT\n")
	b.WriteString(formatEnv())
	b.WriteString("\nEXAMPLES\n")
	b.WriteString(`  mdl https://open.spotify.com/album/...     download into the current directory and record it in the library
                                             (album = this directory's name, artist = the directory above)
  mdl -N https://open.spotify.com/album/...  same, but do not record it
  mdl -b https://music.youtube.com/playlist?list=...   ...then run beet on it
  mdl --youtube https://example.com/...      force yt-dlp (or --spotify to force spotdl)
  mdl -t https://music.youtube.com/playlist?list=...   file it under <Artist>/<Album>/ in the music dir
  mdl config editor "code --wait"           save the editor that mdl edit and mdl -c use
  mdl config audio "bandcamp youtube-music" let spotdl get audio from Bandcamp first
  mdl add -a "Daft Punk" -A Discovery -u https://open.spotify.com/album/...
  mdl list
  mdl tui                                   browse; press d to download the selection, x to delete it
  mdl download                              download every album in the library
  mdl download -a "Daft Punk" -sb           one artist, force Spotify links, then run beet
  mdl download https://music.youtube.com/playlist?list=...
  mdl beet ~/Music/Daft\ Punk               run beet on a folder
  mdl install                               install beet, yt-dlp and spotdl
  mdl -c                                    create ./mdl.json, edit it, import its albums
  mdl import old/mdl.json -a "Some Artist"  import a batch file (artist from -a, else its directory)
`)
	b.WriteString("\nEXIT STATUS\n  0 success   1 a command failed   2 usage error   130 interrupted\n")
	return b.String()
}

func formatEnv() string {
	rows := [][2]string{
		{"MDL_LIBRARY", "library file (default: $XDG_CONFIG_HOME/mdl/library.json or ~/.config/mdl/library.json)"},
		{"MDL_MUSIC_DIR", "where Artist/Album folders are created (default: current directory)"},
		{"MDL_CONFIG", "config file with saved settings (default: $XDG_CONFIG_HOME/mdl/config.json or ~/.config/mdl/config.json)"},
		{"VISUAL, EDITOR", "editor for `mdl edit` and `mdl -c` unless one is saved with `mdl config editor` (default: nano)"},
	}
	var b strings.Builder
	for _, r := range rows {
		lines := wrapWords(r[1], helpWidth-22)
		fmt.Fprintf(&b, "  %-18s%s\n", r[0], lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(&b, "  %-18s%s\n", "", l)
		}
	}
	return b.String()
}

// commandHelp is the full `mdl <command> --help` text.
func (a *App) commandHelp(c *command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mdl %s - %s\n\n", c.Name, c.Summary)
	fmt.Fprintf(&b, "USAGE\n  mdl [GLOBAL OPTIONS] %s", c.Name)
	if len(c.Opts) > 0 {
		b.WriteString(" [OPTIONS]")
	}
	if c.Args != "" {
		b.WriteString(" " + c.Args)
	}
	b.WriteString("\n")
	if len(c.Aliases) > 0 {
		fmt.Fprintf(&b, "\nALIASES\n  %s\n", strings.Join(c.Aliases, ", "))
	}
	b.WriteString("\nDESCRIPTION\n" + paragraph(c.Long))
	if len(c.Opts) > 0 {
		b.WriteString("\nOPTIONS\n" + formatOpts(c.Opts))
	}
	b.WriteString("\nGLOBAL OPTIONS\n" + formatOpts(globalOpts))
	if len(c.Examples) > 0 {
		b.WriteString("\nEXAMPLES\n")
		for _, e := range c.Examples {
			b.WriteString("  " + e + "\n")
		}
	}
	return b.String()
}
