package cli

import (
	"fmt"
	"strings"
)

// Opt describes one command-line option. Every option has a long name and a
// one-letter short name; Arg is the value placeholder ("" for boolean flags).
type Opt struct {
	Short rune
	Long  string
	Arg   string
	Desc  string
}

// Values holds the parsed options. Each occurrence of an option is recorded
// in order, so repeatable options work and the last occurrence wins for
// single-valued lookups.
type Values struct {
	m map[string][]string
}

func newValues() *Values { return &Values{m: map[string][]string{}} }

// Has reports whether the option was given at all.
func (v *Values) Has(long string) bool { return len(v.m[long]) > 0 }

// Bool reports whether a boolean flag was given.
func (v *Values) Bool(long string) bool { return v.Has(long) }

// String returns the last value given for the option, or "".
func (v *Values) String(long string) string {
	vals := v.m[long]
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// Strings returns every value given for a repeatable option.
func (v *Values) Strings(long string) []string { return v.m[long] }

// merge returns a new Values with b's occurrences after a's.
func merge(a, b *Values) *Values {
	out := newValues()
	for _, src := range []*Values{a, b} {
		for k, vals := range src.m {
			out.m[k] = append(out.m[k], vals...)
		}
	}
	return out
}

// longAliases lets an option be spelled with an extra long name. The old
// tool-based names keep working: --spotdl means --spotify, --yt-dlp means
// --youtube. Values are always recorded under the canonical name.
var longAliases = map[string]string{
	"spotdl": "spotify",
	"yt-dlp": "youtube",
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &usageError{fmt.Sprintf(format, a...)} }

// parse understands:
//
//	--long  --long VALUE  --long=VALUE  -s  -s VALUE  -sVALUE  -abc (clusters)  --
//
// Options and positional arguments may be interleaved unless stopAtPositional
// is set, in which case parsing stops at the first positional argument and
// the rest is returned untouched (used to separate global options from the
// command name).
func parse(specs []Opt, args []string, stopAtPositional bool) (*Values, []string, error) {
	vals := newValues()
	var rest []string

	byLong := map[string]Opt{}
	byShort := map[rune]Opt{}
	for _, s := range specs {
		byLong[s.Long] = s
		byShort[s.Short] = s
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return vals, append(rest, args[i+1:]...), nil

		case a == "-" || !strings.HasPrefix(a, "-"):
			if stopAtPositional {
				return vals, append(rest, args[i:]...), nil
			}
			rest = append(rest, a)

		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			if canon, isAlias := longAliases[name]; isAlias {
				if _, exists := byLong[canon]; exists {
					name = canon
				}
			}
			opt, ok := byLong[name]
			if !ok {
				return nil, nil, usagef("unknown option --%s", name)
			}
			if opt.Arg == "" {
				if hasVal {
					return nil, nil, usagef("option --%s does not take a value", name)
				}
				vals.m[name] = append(vals.m[name], "true")
				continue
			}
			if !hasVal {
				if i+1 >= len(args) {
					return nil, nil, usagef("option --%s needs a value (%s)", name, opt.Arg)
				}
				i++
				val = args[i]
			}
			vals.m[name] = append(vals.m[name], val)

		default: // short option or cluster
			cluster := []rune(a[1:])
			for j := 0; j < len(cluster); j++ {
				opt, ok := byShort[cluster[j]]
				if !ok {
					return nil, nil, usagef("unknown option -%c", cluster[j])
				}
				if opt.Arg == "" {
					vals.m[opt.Long] = append(vals.m[opt.Long], "true")
					continue
				}
				val := string(cluster[j+1:])
				if val == "" {
					if i+1 >= len(args) {
						return nil, nil, usagef("option -%c needs a value (%s)", opt.Short, opt.Arg)
					}
					i++
					val = args[i]
				}
				vals.m[opt.Long] = append(vals.m[opt.Long], val)
				break
			}
		}
	}
	return vals, rest, nil
}
