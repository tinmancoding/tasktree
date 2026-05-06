package variable

import (
	"fmt"
	"os"
	"strings"
)

// VariableSource can look up a variable value by name.
type VariableSource interface {
	Get(name string) (value string, found bool)
}

// CLIArgsSource resolves variables from CLI key=value positional arguments.
type CLIArgsSource map[string]string

func (s CLIArgsSource) Get(name string) (string, bool) {
	v, ok := s[name]
	return v, ok
}

// EnvSource resolves variables from environment variables with a prefix.
// Variable "ticket_number" maps to env var "TASKTREE_VAR_TICKET_NUMBER".
type EnvSource struct {
	Prefix string // e.g. "TASKTREE_VAR_"
}

func NewEnvSource() EnvSource {
	return EnvSource{Prefix: "TASKTREE_VAR_"}
}

func (s EnvSource) Get(name string) (string, bool) {
	envKey := s.Prefix + strings.ToUpper(name)
	v := os.Getenv(envKey)
	if v == "" {
		// Distinguish between "not set" and "set to empty".
		_, exists := os.LookupEnv(envKey)
		return v, exists
	}
	return v, true
}

// DefaultsSource resolves variables from parameter-level default values.
type DefaultsSource map[string]string

func (s DefaultsSource) Get(name string) (string, bool) {
	v, ok := s[name]
	return v, ok
}

// Resolver resolves a variable name by consulting a chain of VariableSources
// in priority order (first source wins).
type Resolver struct {
	sources []VariableSource
}

// NewResolver creates a Resolver that checks sources in the provided order.
// Standard resolution order per the design doc:
//  1. CLI args source
//  2. Env source
//  3. Parameter defaults source
//  4. Inline defaults (handled in RenderString via VariableRef.Default)
func NewResolver(sources ...VariableSource) Resolver {
	return Resolver{sources: sources}
}

// Resolve returns the value for name by walking the source chain.
// found is true if any source returned a value.
func (r Resolver) Resolve(name string) (value string, found bool) {
	for _, src := range r.sources {
		if v, ok := src.Get(name); ok {
			return v, true
		}
	}
	return "", false
}

// ResolveAll resolves all vars in refs, returning a map of name→value.
// For each ref:
//   - If a source in the chain provides a value, it is used.
//   - Otherwise, the inline default from the ref is used (if present).
//   - If neither resolves and required is determined by the caller, an error
//     is expected to be returned by the caller after inspecting the map.
func (r Resolver) ResolveAll(refs []VariableRef) map[string]string {
	result := make(map[string]string, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, alreadySeen := seen[ref.Name]; alreadySeen {
			continue
		}
		seen[ref.Name] = struct{}{}
		if v, found := r.Resolve(ref.Name); found {
			result[ref.Name] = v
			continue
		}
		if ref.HasDefault {
			result[ref.Name] = ref.Default
		}
		// If still not found, leave absent from the map — caller handles required check.
	}
	return result
}

// ParseKVArgs parses a slice of "key=value" strings into a CLIArgsSource.
// Returns an error if any element is not in "key=value" format.
func ParseKVArgs(args []string) (CLIArgsSource, error) {
	result := make(CLIArgsSource, len(args))
	for _, arg := range args {
		idx := strings.IndexByte(arg, '=')
		if idx < 0 {
			return nil, fmt.Errorf("variable argument %q must be in key=value format", arg)
		}
		result[arg[:idx]] = arg[idx+1:]
	}
	return result, nil
}
