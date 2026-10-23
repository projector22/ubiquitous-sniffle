package runner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	tests := map[string]string{
		"":                  "''",
		"simple-1.2/x_y":    "simple-1.2/x_y",
		"has space":         "'has space'",
		"it's":              `'it'\''s'`,
		"a&b":               "'a&b'",
		"%(title)s.%(ext)s": "'%(title)s.%(ext)s'",
	}
	for in, want := range tests {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format("", "beet", "import", "-C", "My Dir"); got != "+ beet import -C 'My Dir'" {
		t.Errorf("got %q", got)
	}
	if got := Format("/m/A B", "spotdl", "download", "u"); got != "+ (cd '/m/A B' && spotdl download u)" {
		t.Errorf("got %q", got)
	}
}

func TestDry(t *testing.T) {
	var b bytes.Buffer
	if err := (Dry{Out: &b}).Run(context.Background(), "d", "x", "a b"); err != nil {
		t.Fatal(err)
	}
	if b.String() != "+ (cd d && x 'a b')\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestFake(t *testing.T) {
	boom := errors.New("boom")
	f := &Fake{Err: func(c Call) error {
		if c.Name == "bad" {
			return boom
		}
		return nil
	}}
	if err := f.Run(context.Background(), "d", "ok", "1"); err != nil {
		t.Fatal(err)
	}
	if err := f.Run(context.Background(), "", "bad"); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if len(f.Calls) != 2 || f.Calls[0].Dir != "d" || f.Calls[0].Args[0] != "1" {
		t.Errorf("calls = %+v", f.Calls)
	}
}

func TestExec(t *testing.T) {
	var out, errb bytes.Buffer
	e := Exec{Stdout: &out, Stderr: &errb}
	ctx := context.Background()

	// "go" is guaranteed to exist where these tests run.
	if err := e.Run(ctx, "", "go", "version"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "go version") {
		t.Errorf("stdout = %q", out.String())
	}

	err := e.Run(ctx, "", "mdl-definitely-not-installed-xyz")
	if err == nil || !strings.Contains(err.Error(), "mdl install") {
		t.Errorf("expected install hint, got %v", err)
	}

	err = e.Run(ctx, "", "go", "this-is-not-a-subcommand")
	if err == nil || strings.Contains(err.Error(), "mdl install") {
		t.Errorf("expected a plain failure, got %v", err)
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := e.Run(cctx, "", "go", "version"); err == nil {
		t.Error("expected error from cancelled context")
	}
}
