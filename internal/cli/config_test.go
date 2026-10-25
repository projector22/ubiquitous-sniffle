package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr string
	}{
		{"nano", []string{"nano"}, ""},
		{"  code   --wait ", []string{"code", "--wait"}, ""},
		{"code -w -n", []string{"code", "-w", "-n"}, ""},
		{`"/opt/My Editor/ed" -w`, []string{"/opt/My Editor/ed", "-w"}, ""},
		{`'/opt/My Editor/ed' -w`, []string{"/opt/My Editor/ed", "-w"}, ""},
		{`ed --name="two words"`, []string{"ed", "--name=two words"}, ""},
		{`ed 'it''s'`, []string{"ed", "its"}, ""},
		{`ed "say \"hi\""`, []string{"ed", `say "hi"`}, ""},
		{`ed ""`, []string{"ed", ""}, ""},
		{`C:\tools\edit.exe -w`, []string{`C:\tools\edit.exe`, "-w"}, ""},
		{`"C:\Program Files\Ed\ed.exe" -w`, []string{`C:\Program Files\Ed\ed.exe`, "-w"}, ""},
		{"a\tb\nc", []string{"a", "b", "c"}, ""},
		{"", nil, ""},
		{"   ", nil, ""},
		{`ed 'oops`, nil, "unterminated '"},
		{`ed "oops`, nil, `unterminated "`},
	}
	for _, tc := range tests {
		got, err := splitCommand(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("splitCommand(%q) err = %v, want containing %q", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommand(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

// cfgPath is where the harness keeps its config unless told otherwise.
func (h *harness) cfgPath() string { return filepath.Join(h.dir, ".config", "mdl", "config.json") }

func (h *harness) withEnv(extra map[string]string) {
	env := map[string]string{"MDL_LIBRARY": h.libPath, "MDL_MUSIC_DIR": h.musicDir}
	for k, v := range extra {
		env[k] = v
	}
	h.app.Getenv = func(k string) string { return env[k] }
}

func TestConfigSetGetListUnset(t *testing.T) {
	h := newHarness(t)

	h.expect(t, 0, "config")
	for _, want := range []string{"Config file: " + h.cfgPath() + " (not created yet)", "editor = nano   (default)"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("list missing %q:\n%s", want, h.out)
		}
	}

	h.expect(t, 0, "config", "editor", "code --wait")
	if !strings.Contains(h.out.String(), "Set editor = code --wait in "+h.cfgPath()) {
		t.Errorf("out = %q", h.out)
	}
	// The harness LookPath finds nothing, so the user is warned but it is saved.
	if !strings.Contains(h.err.String(), `"code" was not found on your PATH`) {
		t.Errorf("stderr = %q", h.err)
	}
	if raw, _ := os.ReadFile(h.cfgPath()); string(raw) != "{\n  \"editor\": \"code --wait\"\n}\n" {
		t.Errorf("config file = %q", raw)
	}
	// It lives apart from the library, which was never touched.
	if _, err := os.Stat(h.libPath); err == nil {
		t.Error("setting the editor must not create the library")
	}

	h.expect(t, 0, "config", "editor")
	if h.out.String() != "code --wait\n" {
		t.Errorf("get printed %q", h.out)
	}
	h.expect(t, 0, "config")
	if !strings.Contains(h.out.String(), "editor = code --wait   (config file)") || !strings.Contains(h.out.String(), "(exists)") {
		t.Errorf("list:\n%s", h.out)
	}

	h.expect(t, 0, "config", "--unset", "editor")
	if !strings.Contains(h.out.String(), "Removed editor from "+h.cfgPath()) {
		t.Errorf("out = %q", h.out)
	}
	h.expect(t, 0, "config", "-u", "editor")
	if !strings.Contains(h.out.String(), "editor is not set in") {
		t.Errorf("out = %q", h.out)
	}
	h.expect(t, 0, "config", "editor")
	if h.out.String() != "nano\n" {
		t.Errorf("after unset, get printed %q", h.out)
	}
}

func TestConfigSetNoPathWarningWhenEditorExists(t *testing.T) {
	h := newHarness(t)
	h.app.LookPath = func(n string) (string, error) { return "/usr/bin/" + n, nil }
	h.expect(t, 0, "config", "editor", "nvim")
	if h.err.String() != "" {
		t.Errorf("unexpected stderr %q", h.err)
	}
}

func TestEditorPrecedence(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ran := func(args ...string) string {
		t.Helper()
		h.expect(t, 0, args...)
		return h.fake.Calls[0].Name
	}

	if got := ran("edit"); got != "nano" {
		t.Errorf("default = %s", got)
	}
	h.withEnv(map[string]string{"EDITOR": "ed"})
	if got := ran("edit"); got != "ed" {
		t.Errorf("$EDITOR = %s", got)
	}
	h.withEnv(map[string]string{"EDITOR": "ed", "VISUAL": "visual"})
	if got := ran("edit"); got != "visual" {
		t.Errorf("$VISUAL = %s", got)
	}
	// A saved choice beats both environment variables (like git's core.editor).
	h.expect(t, 0, "config", "editor", "saved --flag")
	if got := ran("edit"); got != "saved" || h.fake.Calls[0].Args[0] != "--flag" {
		t.Errorf("config = %+v", h.fake.Calls[0])
	}
	// --editor beats everything, for this run only.
	if got := ran("edit", "-e", "oneoff"); got != "oneoff" {
		t.Errorf("--editor = %s", got)
	}
	if got := ran("edit"); got != "saved" {
		t.Errorf("--editor must not be remembered, got %s", got)
	}
	// Unsetting falls back to the environment again.
	h.expect(t, 0, "config", "--unset", "editor")
	if got := ran("edit"); got != "visual" {
		t.Errorf("after unset = %s", got)
	}
	// The listing names the source of the value in use.
	h.expect(t, 0, "config")
	if !strings.Contains(h.out.String(), "editor = visual   ($VISUAL)") {
		t.Errorf("list:\n%s", h.out)
	}
}

func TestSavedEditorIsUsedByImportCreateToo(t *testing.T) {
	h := newHarness(t)
	h.chdir(t, "Artist")
	h.expect(t, 0, "config", "editor", "myed -x")
	h.fake.Calls = nil
	h.edit(strp(`{"A": "` + spotURL + `"}`))
	h.expect(t, 0, "-c")
	if c := h.fake.Calls[0]; c.Name != "myed" || !reflect.DeepEqual(c.Args, []string{"-x", "mdl.json"}) {
		t.Errorf("call = %+v", c)
	}
	h.expect(t, 0, "-c", "--editor", "other")
	if h.fake.Calls[0].Name != "other" {
		t.Errorf("import --editor ignored: %+v", h.fake.Calls[0])
	}
}

func TestSavedEditorWithQuotedPath(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.expect(t, 0, "config", "editor", `"/opt/My Editor/ed" --wait`)
	h.expect(t, 0, "edit")
	c := h.fake.Calls[0]
	if c.Name != "/opt/My Editor/ed" || !reflect.DeepEqual(c.Args, []string{"--wait", h.libPath}) {
		t.Errorf("call = %+v", c)
	}
}

func TestConfigFileLocationOptions(t *testing.T) {
	h := newHarness(t)
	custom := filepath.Join(h.dir, "elsewhere", "my-config.json")
	h.expect(t, 0, "-C", custom, "config", "editor", "viaflag")
	if raw, _ := os.ReadFile(custom); !strings.Contains(string(raw), "viaflag") {
		t.Errorf("-C file = %q", raw)
	}
	if _, err := os.Stat(h.cfgPath()); err == nil {
		t.Error("the default config file must not be touched when -C is given")
	}
	h.expect(t, 0, "--config", custom, "config", "editor")
	if h.out.String() != "viaflag\n" {
		t.Errorf("out = %q", h.out)
	}

	envPath := filepath.Join(h.dir, "env-config.json")
	h.withEnv(map[string]string{"MDL_CONFIG": envPath})
	h.expect(t, 0, "config", "editor", "viaenv")
	if raw, _ := os.ReadFile(envPath); !strings.Contains(string(raw), "viaenv") {
		t.Errorf("$MDL_CONFIG file = %q", raw)
	}
	// XDG_CONFIG_HOME applies when MDL_CONFIG is not set.
	xdg := filepath.Join(h.dir, "xdg")
	h.withEnv(map[string]string{"XDG_CONFIG_HOME": xdg})
	h.expect(t, 0, "config", "editor", "viaxdg")
	if _, err := os.Stat(filepath.Join(xdg, "mdl", "config.json")); err != nil {
		t.Errorf("XDG config not written: %v", err)
	}
}

func TestConfigDryRun(t *testing.T) {
	h := newHarness(t)
	h.expect(t, 0, "-n", "config", "editor", "code")
	if !strings.Contains(h.out.String(), "would set editor = code") {
		t.Errorf("out = %q", h.out)
	}
	if _, err := os.Stat(h.cfgPath()); err == nil {
		t.Error("dry run must not write the config")
	}
	h.expect(t, 0, "config", "editor", "code")
	h.expect(t, 0, "-n", "config", "--unset", "editor")
	if !strings.Contains(h.out.String(), "would remove editor") {
		t.Errorf("out = %q", h.out)
	}
	h.expect(t, 0, "config", "editor")
	if h.out.String() != "code\n" {
		t.Errorf("dry-run unset must not remove: %q", h.out)
	}
}

func TestConfigUsageErrors(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"config", "nope"},
		{"config", "nope", "x"},
		{"config", "--unset", "nope"},
		{"config", "--unset"},
		{"config", "--unset", "editor", "extra"},
		{"config", "editor", "a", "b"},
		{"config", "editor", "   "},
		{"config", "editor", `"unterminated`},
	} {
		if code := h.run(args...); code != 2 {
			t.Errorf("mdl %q: exit %d, want 2 (stderr %q)", args, code, h.err)
		}
	}
	if _, err := os.Stat(h.cfgPath()); err == nil {
		t.Error("a rejected command must not create the config file")
	}
}

func TestBrokenConfigIsReportedNotOverwritten(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	_ = os.MkdirAll(filepath.Dir(h.cfgPath()), 0o755)
	broken := []byte(`{"editr": "typo"}`)
	_ = os.WriteFile(h.cfgPath(), broken, 0o644)

	for _, args := range [][]string{{"config"}, {"config", "editor"}, {"config", "editor", "new"}, {"config", "-u", "editor"}, {"edit"}} {
		h.expect(t, 1, args...)
		if !strings.Contains(h.err.String(), h.cfgPath()) {
			t.Errorf("mdl %v: error should name the config file: %q", args, h.err)
		}
	}
	if raw, _ := os.ReadFile(h.cfgPath()); string(raw) != string(broken) {
		t.Errorf("a broken config must never be overwritten, got %q", raw)
	}
	// An explicit --editor does not need the config at all.
	h.expect(t, 0, "edit", "--editor", "vim")
	if h.fake.Calls[0].Name != "vim" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
}

func TestInvalidSavedEditorCommand(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	_ = os.MkdirAll(filepath.Dir(h.cfgPath()), 0o755)
	_ = os.WriteFile(h.cfgPath(), []byte(`{"editor": "'oops"}`), 0o644)
	h.expect(t, 1, "edit")
	if !strings.Contains(h.err.String(), "invalid editor command") || !strings.Contains(h.err.String(), "config file") {
		t.Errorf("stderr = %q", h.err)
	}
	// Same through import -c, which never opened the editor.
	h.chdir(t, "A")
	h.expect(t, 1, "-c")
	if len(h.fake.Calls) != 0 {
		t.Errorf("nothing should have run: %+v", h.fake.Calls)
	}
	// A blank value (only whitespace in $EDITOR) is treated as unset.
	_ = os.Remove(h.cfgPath())
	h.withEnv(map[string]string{"EDITOR": "   "})
	h.expect(t, 0, "edit")
	if h.fake.Calls[0].Name != "nano" {
		t.Errorf("call = %+v", h.fake.Calls[0])
	}
}

func TestConfigSaveFailure(t *testing.T) {
	h := newHarness(t)
	blocker := filepath.Join(h.dir, "file")
	_ = os.WriteFile(blocker, nil, 0o644)
	bad := filepath.Join(blocker, "sub", "config.json") // a "directory" that is really a file
	h.expect(t, 1, "-C", bad, "config", "editor", "x")
	if !strings.Contains(h.err.String(), bad) {
		t.Errorf("the error should name the path: %q", h.err)
	}
	h.expect(t, 1, "-C", bad, "config")
}

func TestHelpDocumentsConfig(t *testing.T) {
	a := &App{}
	top := a.topHelp()
	for _, want := range []string{"config", "MDL_CONFIG", "--config", "nano", "mdl config editor"} {
		if !strings.Contains(top, want) {
			t.Errorf("top help missing %q", want)
		}
	}
	if strings.Contains(top, "default: vi)") {
		t.Error("help still claims vi is the default editor")
	}
	for _, name := range []string{"config", "edit", "import"} {
		help := strings.Join(strings.Fields(a.commandHelp(a.find(name))), " ")
		if !strings.Contains(help, "--editor") && name != "config" {
			t.Errorf("%s help should document --editor", name)
		}
		if !strings.Contains(help, "mdl config") {
			t.Errorf("%s help should point at `mdl config`", name)
		}
	}
}
