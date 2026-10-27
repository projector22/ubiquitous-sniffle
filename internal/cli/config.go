package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/projector22/ubiquitous-sniffle/internal/config"
)

// defaultEditor is used when nothing else names an editor.
const defaultEditor = "nano"

// editorOpt is shared by every command that opens an editor.
var editorOpt = Opt{'e', "editor", "CMD", "Editor command for this run, overriding the config file, $VISUAL and $EDITOR. May include arguments and quotes, e.g. \"code --wait\"."}

// setting is a resolved setting together with where it came from.
type setting struct {
	Value, Source string
}

// editorSetting resolves the editor command and reports its origin, in this
// order: --editor, the config file, $VISUAL, $EDITOR, then nano. (Like git, a
// choice saved in the tool's own config wins over the environment.) The
// config file is only read when --editor is not given, so a broken config
// never stops an explicit --editor.
func (a *App) editorSetting(inv *invocation) (setting, error) {
	if v := strings.TrimSpace(inv.vals.String("editor")); v != "" {
		return setting{v, "--editor"}, nil
	}
	cfg, err := config.Load(inv.configPath)
	if err != nil {
		return setting{}, err
	}
	if v := strings.TrimSpace(cfg.Editor); v != "" {
		return setting{v, "config file"}, nil
	}
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(a.Getenv(name)); v != "" {
			return setting{v, "$" + name}, nil
		}
	}
	return setting{defaultEditor, "default"}, nil
}

// editorCommand returns the editor as program + leading arguments.
func (a *App) editorCommand(inv *invocation) ([]string, error) {
	s, err := a.editorSetting(inv)
	if err != nil {
		return nil, err
	}
	parts, err := splitCommand(s.Value)
	if err != nil || len(parts) == 0 {
		if err == nil {
			err = errors.New("empty command")
		}
		return nil, fmt.Errorf("invalid editor command %q (from %s): %w", s.Value, s.Source, err)
	}
	return parts, nil
}

// splitCommand splits a command line into words. Whitespace separates words;
// single quotes keep everything literally; double quotes keep whitespace and
// allow \" for a quote. A backslash anywhere else is an ordinary character,
// so Windows paths such as C:\tools\edit.exe work unquoted.
func splitCommand(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case '\'':
			end := indexRune(rs, '\'', i+1)
			if end < 0 {
				return nil, errors.New("unterminated ' quote")
			}
			cur.WriteString(string(rs[i+1 : end]))
			i, inWord = end, true
		case '"':
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && rs[j+1] == '"' {
					j++
					cur.WriteRune('"')
					continue
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated \" quote")
			}
			i, inWord = j, true
		case ' ', '\t', '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

func indexRune(rs []rune, want rune, from int) int {
	for i := from; i < len(rs); i++ {
		if rs[i] == want {
			return i
		}
	}
	return -1
}

// settingFor resolves any known setting. Only the editor exists so far.
func (a *App) settingFor(inv *invocation, key string) (setting, error) {
	switch key {
	case config.KeyEditor:
		return a.editorSetting(inv)
	}
	return setting{}, fmt.Errorf("unknown setting %q", key)
}

func (a *App) cmdConfig(inv *invocation) int {
	unset := inv.vals.Bool("unset")
	switch {
	case len(inv.args) > 2:
		return a.usageErr(errors.New("config: expected KEY [VALUE]; quote a VALUE that contains spaces"))
	case unset && len(inv.args) != 1:
		return a.usageErr(errors.New("config: --unset takes exactly one KEY"))
	}
	if len(inv.args) == 0 {
		return a.listConfig(inv)
	}
	key := inv.args[0]
	if !config.Known(key) {
		return a.usageErr(fmt.Errorf("config: unknown setting %q (known: %s)", key, strings.Join(config.Keys, ", ")))
	}

	switch {
	case unset:
		return a.unsetConfig(inv, key)
	case len(inv.args) == 2:
		return a.setConfig(inv, key, inv.args[1])
	}
	s, err := a.settingFor(inv, key)
	if err != nil {
		return a.fail("%v", err)
	}
	fmt.Fprintln(a.Stdout, s.Value)
	return exitOK
}

func (a *App) listConfig(inv *invocation) int {
	state := "not created yet"
	if _, err := config.Load(inv.configPath); err != nil {
		return a.fail("%v", err)
	}
	if fileExists(inv.configPath) {
		state = "exists"
	}
	fmt.Fprintf(a.Stdout, "Config file: %s (%s)\n", inv.configPath, state)
	for _, key := range config.Keys {
		s, err := a.settingFor(inv, key)
		if err != nil {
			return a.fail("%v", err)
		}
		fmt.Fprintf(a.Stdout, "  %s = %s   (%s)\n", key, s.Value, s.Source)
	}
	return exitOK
}

func (a *App) setConfig(inv *invocation, key, value string) int {
	if key == config.KeyEditor {
		parts, err := splitCommand(value)
		if err != nil || len(parts) == 0 {
			if err == nil {
				err = errors.New("empty command")
			}
			return a.usageErr(fmt.Errorf("config: invalid editor command %q: %w", value, err))
		}
		if _, err := a.LookPath(parts[0]); err != nil {
			fmt.Fprintf(a.Stderr, "mdl: note: %q was not found on your PATH (saving it anyway)\n", parts[0])
		}
	}
	// A config file that cannot be read is reported rather than overwritten.
	cfg, err := config.Load(inv.configPath)
	if err != nil {
		return a.fail("%v", err)
	}
	if err := cfg.Set(key, value); err != nil {
		return a.usageErr(fmt.Errorf("config: %w", err))
	}
	if inv.dry {
		fmt.Fprintf(a.Stdout, "would set %s = %s in %s\n", key, strings.TrimSpace(value), inv.configPath)
		return exitOK
	}
	if err := cfg.Save(inv.configPath); err != nil {
		return a.fail("saving config: %v", err)
	}
	fmt.Fprintf(a.Stdout, "Set %s = %s in %s\n", key, strings.TrimSpace(value), inv.configPath)
	return exitOK
}

func (a *App) unsetConfig(inv *invocation, key string) int {
	cfg, err := config.Load(inv.configPath)
	if err != nil {
		return a.fail("%v", err)
	}
	had, err := cfg.Unset(key)
	if err != nil {
		return a.usageErr(fmt.Errorf("config: %w", err))
	}
	if !had {
		fmt.Fprintf(a.Stdout, "%s is not set in %s\n", key, inv.configPath)
		return exitOK
	}
	if inv.dry {
		fmt.Fprintf(a.Stdout, "would remove %s from %s\n", key, inv.configPath)
		return exitOK
	}
	if err := cfg.Save(inv.configPath); err != nil {
		return a.fail("saving config: %v", err)
	}
	fmt.Fprintf(a.Stdout, "Removed %s from %s\n", key, inv.configPath)
	return exitOK
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
