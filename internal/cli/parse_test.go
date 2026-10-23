package cli

import (
	"reflect"
	"strings"
	"testing"
)

var testSpecs = []Opt{
	{'v', "verbose", "", "verbose"},
	{'q', "quiet", "", "quiet"},
	{'a', "artist", "NAME", "artist"},
	{'u', "url", "URL", "url (repeatable)"},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		stop    bool
		bools   []string
		strs    map[string][]string
		rest    []string
		wantErr string
	}{
		{name: "empty"},
		{name: "long bool", args: []string{"--verbose"}, bools: []string{"verbose"}},
		{name: "short bool", args: []string{"-v"}, bools: []string{"verbose"}},
		{name: "cluster", args: []string{"-vq"}, bools: []string{"verbose", "quiet"}},
		{name: "long value space", args: []string{"--artist", "Daft Punk"}, strs: map[string][]string{"artist": {"Daft Punk"}}},
		{name: "long value equals", args: []string{"--artist=Daft Punk"}, strs: map[string][]string{"artist": {"Daft Punk"}}},
		{name: "equals keeps later equals", args: []string{"--url=a=b"}, strs: map[string][]string{"url": {"a=b"}}},
		{name: "short value space", args: []string{"-a", "X"}, strs: map[string][]string{"artist": {"X"}}},
		{name: "short value attached", args: []string{"-aX"}, strs: map[string][]string{"artist": {"X"}}},
		{name: "cluster ending in value", args: []string{"-vaX"}, bools: []string{"verbose"}, strs: map[string][]string{"artist": {"X"}}},
		{name: "cluster value from next arg", args: []string{"-va", "X"}, bools: []string{"verbose"}, strs: map[string][]string{"artist": {"X"}}},
		{name: "repeatable", args: []string{"-u", "a", "--url", "b", "--url=c"}, strs: map[string][]string{"url": {"a", "b", "c"}}},
		{name: "interleaved positionals", args: []string{"one", "-v", "two"}, bools: []string{"verbose"}, rest: []string{"one", "two"}},
		{name: "double dash", args: []string{"-v", "--", "-q", "--artist"}, bools: []string{"verbose"}, rest: []string{"-q", "--artist"}},
		{name: "lone dash is positional", args: []string{"-"}, rest: []string{"-"}},
		{name: "value may look like an option", args: []string{"--artist", "-x"}, strs: map[string][]string{"artist": {"-x"}}},
		{name: "stop at positional", args: []string{"-v", "cmd", "-q"}, stop: true, bools: []string{"verbose"}, rest: []string{"cmd", "-q"}},
		{name: "stop at double dash", args: []string{"--", "-v"}, stop: true, rest: []string{"-v"}},
		{name: "unknown long", args: []string{"--nope"}, wantErr: "unknown option --nope"},
		{name: "unknown short", args: []string{"-z"}, wantErr: "unknown option -z"},
		{name: "unknown in cluster", args: []string{"-vz"}, wantErr: "unknown option -z"},
		{name: "missing long value", args: []string{"--artist"}, wantErr: "--artist needs a value"},
		{name: "missing short value", args: []string{"-a"}, wantErr: "-a needs a value"},
		{name: "bool with value", args: []string{"--verbose=1"}, wantErr: "does not take a value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, rest, err := parse(testSpecs, tc.args, tc.stop)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				var ue *usageError
				if !strings.Contains(err.Error(), tc.wantErr) || !asUsage(err, &ue) {
					t.Errorf("error should be a *usageError: %T", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range tc.bools {
				if !v.Bool(b) {
					t.Errorf("flag %s not set", b)
				}
			}
			for k, want := range tc.strs {
				if got := v.Strings(k); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %v, want %v", k, got, want)
				}
				if got := v.String(k); got != want[len(want)-1] {
					t.Errorf("String(%s) = %q", k, got)
				}
			}
			if len(rest) != len(tc.rest) || (len(rest) > 0 && !reflect.DeepEqual(rest, tc.rest)) {
				t.Errorf("rest = %v, want %v", rest, tc.rest)
			}
		})
	}
}

func asUsage(err error, target **usageError) bool {
	ue, ok := err.(*usageError)
	*target = ue
	return ok
}

func TestValuesAccessors(t *testing.T) {
	v := newValues()
	if v.Has("x") || v.Bool("x") || v.String("x") != "" || v.Strings("x") != nil {
		t.Error("empty Values should report nothing")
	}
	a, b := newValues(), newValues()
	a.m["k"] = []string{"1"}
	b.m["k"] = []string{"2"}
	b.m["z"] = []string{"true"}
	m := merge(a, b)
	if m.String("k") != "2" || !reflect.DeepEqual(m.Strings("k"), []string{"1", "2"}) || !m.Bool("z") {
		t.Errorf("merge = %v", m.m)
	}
	if len(a.m["k"]) != 1 {
		t.Error("merge must not mutate its inputs")
	}
}
