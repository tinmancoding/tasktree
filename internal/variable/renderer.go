package variable

import (
	"strings"
)

// RenderString replaces all {{variable}} and {{variable | default:val}}
// references in s with the corresponding value from values.
// If a variable is not found in values, the reference is left as-is
// (callers should validate completeness before or after rendering).
func RenderString(s string, values map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return varRefRe.ReplaceAllStringFunc(s, func(match string) string {
		submatches := varRefRe.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		name := submatches[1]
		if v, ok := values[name]; ok {
			return v
		}
		return match
	})
}

// RenderStringMap applies RenderString to every value in a map, returning
// a new map with the same keys and rendered values.
func RenderStringMap(m map[string]string, values map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = RenderString(v, values)
	}
	return result
}
