package cli

import (
	"strings"
	"testing"
)

func TestWrapWords(t *testing.T) {
	got := wrapWords("one two three four five", 9)
	want := []string{"one two", "three", "four five"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v", got)
	}
	if len(wrapWords("", 10)) != 0 {
		t.Error("empty text should yield no lines")
	}
	if got := wrapWords("averyveryverylongword x", 5); len(got) != 2 {
		t.Errorf("long word should not be split: %v", got)
	}
}

func TestFormatOpts(t *testing.T) {
	out := formatOpts([]Opt{
		{'a', "artist", "NAME", "short"},
		{'x', "extremely-long", "", strings.Repeat("word ", 30)},
		{'e', "empty", "", ""},
	})
	if !strings.Contains(out, "  -a, --artist NAME      short\n") {
		t.Errorf("alignment wrong:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) > helpWidth {
			t.Errorf("line too long (%d): %q", len(line), line)
		}
	}
	if !strings.Contains(out, "-e, --empty") {
		t.Errorf("option without description missing:\n%s", out)
	}
}

func TestTopHelpMentionsEverything(t *testing.T) {
	a := &App{}
	help := a.topHelp()
	for _, c := range a.commands() {
		if !strings.Contains(help, c.Name) {
			t.Errorf("top help does not mention command %q", c.Name)
		}
	}
	for _, o := range globalOpts {
		if !strings.Contains(help, "--"+o.Long) || !strings.Contains(help, "-"+string(o.Short)+",") {
			t.Errorf("top help does not document %s", o.Long)
		}
	}
	for _, want := range []string{"LIBRARY FORMAT", "MDL_LIBRARY", "MDL_MUSIC_DIR", "EXAMPLES", "EXIT STATUS", `"spotify"`, "-sb"} {
		if !strings.Contains(help, want) {
			t.Errorf("top help missing %q", want)
		}
	}
}

func TestCommandHelpDocumentsEveryOption(t *testing.T) {
	a := &App{}
	for _, c := range a.commands() {
		c := c
		help := a.commandHelp(&c)
		if !strings.HasPrefix(help, "mdl "+c.Name) {
			t.Errorf("%s: help should start with the command name", c.Name)
		}
		for _, o := range c.Opts {
			if !strings.Contains(help, "-"+string(o.Short)+", --"+o.Long) {
				t.Errorf("%s: help does not document --%s", c.Name, o.Long)
			}
		}
		if len(c.Aliases) > 0 && !strings.Contains(help, "ALIASES") {
			t.Errorf("%s: aliases missing from help", c.Name)
		}
	}
}

// Every option, everywhere, must have both a one-letter and a long name, and
// short names must be unique within a command (including the global options).
func TestOptionsAreConsistent(t *testing.T) {
	a := &App{}
	for _, c := range a.commands() {
		short, long := map[rune]string{}, map[string]bool{}
		for _, o := range append(append([]Opt{}, c.Opts...), globalOpts...) {
			if o.Short == 0 || o.Long == "" || o.Desc == "" {
				t.Errorf("%s: option %+v needs a short name, long name and description", c.Name, o)
			}
			if prev, dup := short[o.Short]; dup {
				t.Errorf("%s: -%c used by both --%s and --%s", c.Name, o.Short, prev, o.Long)
			}
			if long[o.Long] {
				t.Errorf("%s: --%s defined twice", c.Name, o.Long)
			}
			short[o.Short], long[o.Long] = o.Long, true
		}
	}
}
