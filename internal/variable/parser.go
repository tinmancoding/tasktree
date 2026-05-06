// Package variable provides variable parsing, resolution, and rendering for
// TaskTree templates. Template bodies may contain {{variable}} or
// {{variable | default:value}} references that are substituted at workspace
// creation time.
package variable

import (
	"fmt"
	"regexp"
	"strings"
)

// varRefRe matches {{name}} and {{name | default:value}} patterns.
// Group 1: variable name
// Group 2: default value (may be empty string if no default clause)
var varRefRe = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\s*(?:\|\s*default:([^}]*))?\}\}`)

// varNameRe validates a variable name: [a-z][a-z0-9_]*
var varNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// VariableRef represents a single parsed {{variable}} reference.
type VariableRef struct {
	Name       string // variable name, e.g. "ticket_number"
	Default    string // inline default value (empty if not specified)
	HasDefault bool   // true when a default clause is present (even if value is empty)
	Raw        string // the full matched string, e.g. "{{ticket_number | default:main}}"
}

// Parse extracts all VariableRef occurrences from s in the order they appear.
// It does not validate names against a parameter list.
func Parse(s string) []VariableRef {
	matches := varRefRe.FindAllStringSubmatchIndex(s, -1)
	refs := make([]VariableRef, 0, len(matches))
	for _, m := range matches {
		raw := s[m[0]:m[1]]
		name := s[m[2]:m[3]]
		var defaultVal string
		hasDefault := m[4] >= 0
		if hasDefault {
			defaultVal = strings.TrimSpace(s[m[4]:m[5]])
		}
		refs = append(refs, VariableRef{
			Name:       name,
			Default:    defaultVal,
			HasDefault: hasDefault,
			Raw:        raw,
		})
	}
	return refs
}

// ValidateName returns an error when name is not a valid variable identifier.
func ValidateName(name string) error {
	if !varNameRe.MatchString(name) {
		return fmt.Errorf("invalid variable name %q (must match [a-z][a-z0-9_]*)", name)
	}
	return nil
}
