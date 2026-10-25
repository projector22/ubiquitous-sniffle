// Package config holds mdl's user settings: a small JSON file kept apart from
// the library so that preferences and data never share a file.
//
//	{
//	  "editor": "code --wait"
//	}
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mdl/internal/atomicfile"
)

// Setting names accepted by Get, Set and Unset.
const (
	KeyEditor = "editor"
)

// Keys lists every known setting.
var Keys = []string{KeyEditor}

// Config is the stored settings. The zero value means "nothing configured".
type Config struct {
	// Editor is the command (with optional arguments) used to edit files.
	Editor string `json:"editor,omitempty"`
}

// Known reports whether key is a setting name.
func Known(key string) bool {
	_, known := (&Config{}).Get(key)
	return known
}

// DefaultPath returns where the config file lives when -C is not given:
// $MDL_CONFIG, else $XDG_CONFIG_HOME/mdl/config.json, else
// <home>/.config/mdl/config.json. It sits beside, but is separate from, the
// library file.
func DefaultPath(getenv func(string) string, home string) string {
	if p := getenv("MDL_CONFIG"); p != "" {
		return p
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mdl", "config.json")
	}
	if home == "" {
		return "config.json"
	}
	return filepath.Join(home, ".config", "mdl", "config.json")
}

// Load reads the config at path. A missing or empty file is not an error: it
// yields an empty Config. Unknown keys are rejected so typos are noticed.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		if errors.Is(err, io.EOF) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("%s: invalid config: %w", path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: invalid config: unexpected data after the top-level object", path)
	}
	return &c, nil
}

// Save writes the config to path atomically, creating parent directories.
func (c *Config) Save(path string) error {
	return atomicfile.Write(path, 0o644, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(c)
	})
}

// Get returns the stored value of key ("" if unset). known is false for a
// key that does not exist.
func (c *Config) Get(key string) (value string, known bool) {
	switch key {
	case KeyEditor:
		return c.Editor, true
	}
	return "", false
}

// Set stores value under key. Values are trimmed and must not be empty; use
// Unset to remove a setting.
func (c *Config) Set(key, value string) error {
	value = strings.TrimSpace(value)
	if _, known := c.Get(key); !known {
		return unknownKey(key)
	}
	if value == "" {
		return fmt.Errorf("%s cannot be empty (use --unset to remove it)", key)
	}
	switch key {
	case KeyEditor:
		c.Editor = value
	}
	return nil
}

// Unset removes key and reports whether it had a value.
func (c *Config) Unset(key string) (had bool, err error) {
	old, known := c.Get(key)
	if !known {
		return false, unknownKey(key)
	}
	switch key {
	case KeyEditor:
		c.Editor = ""
	}
	return old != "", nil
}

func unknownKey(key string) error {
	return fmt.Errorf("unknown setting %q (known: %s)", key, strings.Join(Keys, ", "))
}
