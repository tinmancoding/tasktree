// Package variable implements template variable parsing, resolution, and rendering.
package variable

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var refRE = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\s*(?:\|\s*default:([^}]*))?\}\}`)
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Ref struct {
	Name, Default, Raw string
	HasDefault         bool
}

func Parse(s string) []Ref {
	matches := refRE.FindAllStringSubmatchIndex(s, -1)
	out := make([]Ref, 0, len(matches))
	for _, m := range matches {
		r := Ref{Name: s[m[2]:m[3]], Raw: s[m[0]:m[1]], HasDefault: m[4] >= 0}
		if r.HasDefault {
			r.Default = strings.TrimSpace(s[m[4]:m[5]])
		}
		out = append(out, r)
	}
	return out
}
func ValidateName(s string) error {
	if !nameRE.MatchString(s) {
		return fmt.Errorf("invalid variable name %q (must match [a-z][a-z0-9_]*)", s)
	}
	return nil
}
func ParseKVArgs(args []string) (map[string]string, error) {
	out := map[string]string{}
	for _, arg := range args {
		k, v, ok := strings.Cut(arg, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("variable argument %q must be in key=value format", arg)
		}
		out[k] = v
	}
	return out, nil
}
func Resolve(names []Ref, cli, defaults map[string]string) map[string]string {
	out := map[string]string{}
	for _, r := range names {
		if _, ok := out[r.Name]; ok {
			continue
		}
		if v, ok := cli[r.Name]; ok {
			out[r.Name] = v
			continue
		}
		if v, ok := os.LookupEnv("TASKTREE_VAR_" + strings.ToUpper(r.Name)); ok {
			out[r.Name] = v
			continue
		}
		if v, ok := defaults[r.Name]; ok {
			out[r.Name] = v
			continue
		}
		if r.HasDefault {
			out[r.Name] = r.Default
		}
	}
	return out
}
func Render(s string, values map[string]string) string {
	return refRE.ReplaceAllStringFunc(s, func(m string) string {
		r := Parse(m)
		if len(r) > 0 {
			if v, ok := values[r[0].Name]; ok {
				return v
			}
		}
		return m
	})
}
