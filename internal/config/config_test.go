package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		env  map[string]string
		home string
		want string
	}{
		{map[string]string{"MDL_CONFIG": "/x/c.json", "XDG_CONFIG_HOME": "/cfg"}, "/home/u", "/x/c.json"},
		{map[string]string{"XDG_CONFIG_HOME": "/cfg"}, "/home/u", filepath.Join("/cfg", "mdl", "config.json")},
		{nil, "/home/u", filepath.Join("/home/u", ".config", "mdl", "config.json")},
		{nil, "", "config.json"},
	}
	for _, tc := range tests {
		if got := DefaultPath(env(tc.env), tc.home); got != tc.want {
			t.Errorf("DefaultPath = %q, want %q", got, tc.want)
		}
	}
	// The config file is never the library file.
	if filepath.Base(DefaultPath(env(nil), "/h")) == "library.json" {
		t.Error("config and library must be different files")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c := &Config{}
	if err := c.Set(KeyEditor, "  code --wait  "); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "{\n  \"editor\": \"code --wait\"\n}\n" {
		t.Errorf("file = %q", raw)
	}
	back, err := Load(path)
	if err != nil || back.Editor != "code --wait" {
		t.Errorf("Load = %+v, %v", back, err)
	}
	// An empty config saves as an empty object (not null).
	if err := (&Config{}).Save(path); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "{}\n" {
		t.Errorf("empty file = %q", raw)
	}
}

func TestLoadVariants(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, content := range map[string]string{"empty": "", "blank": " \n", "null": "null", "obj": "{}"} {
		if c, err := Load(write(name, content)); err != nil || c.Editor != "" {
			t.Errorf("%s: %+v %v", name, c, err)
		}
	}
	if c, err := Load(filepath.Join(dir, "missing.json")); err != nil || c.Editor != "" {
		t.Errorf("missing file should be an empty config: %+v %v", c, err)
	}
	for name, tc := range map[string]struct{ content, want string }{
		"typo":     {`{"editr": "x"}`, "unknown field"},
		"wrong":    {`{"editor": 5}`, "invalid config"},
		"trailing": {`{} {}`, "unexpected data"},
		"garbage":  {`nope`, "invalid config"},
	} {
		p := write(name, tc.content)
		if _, err := Load(p); err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), p) {
			t.Errorf("%s: err = %v, want containing %q and the path", name, err, tc.want)
		}
	}
	// A directory where the file should be is a real error, not "missing".
	if _, err := Load(dir); err == nil {
		t.Error("expected an error reading a directory")
	}
}

func TestGetSetUnset(t *testing.T) {
	c := &Config{}
	if v, known := c.Get(KeyEditor); v != "" || !known {
		t.Errorf("Get = %q %v", v, known)
	}
	if _, known := c.Get("nope"); known {
		t.Error("unknown key reported as known")
	}
	if err := c.Set("nope", "x"); err == nil || !strings.Contains(err.Error(), "known: editor") {
		t.Errorf("Set unknown: %v", err)
	}
	if err := c.Set(KeyEditor, "   "); err == nil || !strings.Contains(err.Error(), "--unset") {
		t.Errorf("Set empty: %v", err)
	}
	_ = c.Set(KeyEditor, "nvim")
	if had, err := c.Unset(KeyEditor); !had || err != nil || c.Editor != "" {
		t.Errorf("Unset = %v %v, editor %q", had, err, c.Editor)
	}
	if had, err := c.Unset(KeyEditor); had || err != nil {
		t.Errorf("second Unset = %v %v", had, err)
	}
	if _, err := c.Unset("nope"); err == nil {
		t.Error("Unset unknown should fail")
	}
}

func TestKnown(t *testing.T) {
	if !Known(KeyEditor) || Known("nope") || Known("") {
		t.Error("Known is wrong")
	}
}
