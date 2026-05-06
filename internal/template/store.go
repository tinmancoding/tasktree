// Package template provides template loading, discovery, and validation for
// TaskTree workspace templates (kind: Template).
package template

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/variable"
)

//go:embed builtins/*.yml
var builtinFS embed.FS

// Store discovers, loads, and validates workspace templates.
//
// Discovery order (first match wins per name):
//  1. Project-local:  ./.tasktree/templates/*.yml
//  2. User-level:     ~/.config/tasktree/templates/*.yml
//  3. Built-in:       embedded in the binary
type Store struct {
	projectDir string // optional; "./.tasktree/templates" relative to this
	userDir    string // optional override for user-level template dir
}

// NewStore creates a Store using standard discovery paths.
// projectDir should be the directory that contains .tasktree/ (typically the
// workspace root, or the cwd). Pass "" to skip project-local discovery.
func NewStore(projectDir string) (Store, error) {
	userDir, err := defaultUserTemplateDir()
	if err != nil {
		return Store{}, fmt.Errorf("resolve user template dir: %w", err)
	}
	return Store{
		projectDir: projectDir,
		userDir:    userDir,
	}, nil
}

// newStoreWithUserDir is used in tests to override the user-level directory.
func newStoreWithUserDir(projectDir, userDir string) Store {
	return Store{projectDir: projectDir, userDir: userDir}
}

// NewStoreForTest creates a Store with explicit projectDir and userDir for
// use in tests. Pass "" to disable a discovery path.
func NewStoreForTest(projectDir, userDir string) Store {
	return Store{projectDir: projectDir, userDir: userDir}
}

func defaultUserTemplateDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tasktree", "templates"), nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, ".config", "tasktree", "templates"), nil
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(cfgDir, "tasktree", "templates"), nil
}

// LoadByName finds the first template whose metadata.name matches name,
// searching discovery paths in priority order.
// Returns domain.TemplateNotFoundError when no match is found.
func (s Store) LoadByName(name string) (domain.TemplateSpec, error) {
	specs, err := s.List()
	if err != nil {
		return domain.TemplateSpec{}, err
	}
	for _, spec := range specs {
		if spec.Metadata.Name == name {
			return spec, nil
		}
	}
	return domain.TemplateSpec{}, domain.TemplateNotFoundError{Name: name}
}

// LoadByPath loads a template from an explicit file path.
func (s Store) LoadByPath(path string) (domain.TemplateSpec, error) {
	return loadFile(path)
}

// Load loads a template by name or path. If nameOrPath contains a path
// separator or ends in ".yml" / ".yaml" it is treated as a file path;
// otherwise it is treated as a name to look up via discovery.
func (s Store) Load(nameOrPath string) (domain.TemplateSpec, error) {
	if looksLikePath(nameOrPath) {
		return s.LoadByPath(nameOrPath)
	}
	return s.LoadByName(nameOrPath)
}

// List returns all templates found across all discovery paths.
// Templates from earlier paths shadow same-named templates from later paths.
func (s Store) List() ([]domain.TemplateSpec, error) {
	seen := make(map[string]struct{})
	var result []domain.TemplateSpec

	// 1. Project-local
	if s.projectDir != "" {
		dir := filepath.Join(s.projectDir, ".tasktree", "templates")
		entries, err := loadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, spec := range entries {
			if _, dup := seen[spec.Metadata.Name]; !dup {
				seen[spec.Metadata.Name] = struct{}{}
				result = append(result, spec)
			}
		}
	}

	// 2. User-level
	if s.userDir != "" {
		entries, err := loadDir(s.userDir)
		if err != nil {
			return nil, err
		}
		for _, spec := range entries {
			if _, dup := seen[spec.Metadata.Name]; !dup {
				seen[spec.Metadata.Name] = struct{}{}
				result = append(result, spec)
			}
		}
	}

	// 3. Built-ins
	builtins, err := loadBuiltins()
	if err != nil {
		return nil, err
	}
	for _, spec := range builtins {
		if _, dup := seen[spec.Metadata.Name]; !dup {
			seen[spec.Metadata.Name] = struct{}{}
			result = append(result, spec)
		}
	}

	return result, nil
}

// Validate checks a TemplateSpec for structural correctness:
//   - kind must be "Template"
//   - metadata.name must be non-empty
//   - all parameter names must be valid identifiers
//   - all {{variable}} references in the template body must be declared in parameters
//   - no variable name used in the body is misspelled (typo detection)
func (s Store) Validate(spec domain.TemplateSpec) error {
	if spec.Kind != domain.KindTemplate {
		return fmt.Errorf("invalid kind %q: expected %q", spec.Kind, domain.KindTemplate)
	}
	if spec.Metadata.Name == "" {
		return fmt.Errorf("metadata.name must not be empty")
	}

	// Validate parameter names.
	paramNames := make(map[string]struct{}, len(spec.Parameters))
	for _, p := range spec.Parameters {
		if err := variable.ValidateName(p.Name); err != nil {
			return err
		}
		paramNames[p.Name] = struct{}{}
	}

	// Extract all variable references from the template body.
	refs := extractTemplateRefs(spec.Template)
	for _, ref := range refs {
		if err := variable.ValidateName(ref.Name); err != nil {
			return domain.InvalidVariableNameError{Name: ref.Name}
		}
		if _, declared := paramNames[ref.Name]; !declared {
			suggestion := suggest(ref.Name, paramNames)
			return domain.UnknownVariableError{Name: ref.Name, Suggestion: suggestion}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func loadDir(dir string) ([]domain.TemplateSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read template dir %s: %w", dir, err)
	}
	var specs []domain.TemplateSpec
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		spec, err := loadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("load template %s: %w", name, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func loadBuiltins() ([]domain.TemplateSpec, error) {
	entries, err := builtinFS.ReadDir("builtins")
	if err != nil {
		return nil, fmt.Errorf("read built-in templates: %w", err)
	}
	var specs []domain.TemplateSpec
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile("builtins/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read built-in %s: %w", e.Name(), err)
		}
		spec, err := parseTemplate(data)
		if err != nil {
			return nil, fmt.Errorf("parse built-in %s: %w", e.Name(), err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func loadFile(path string) (domain.TemplateSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.TemplateSpec{}, fmt.Errorf("read template file %s: %w", path, err)
	}
	spec, err := parseTemplate(data)
	if err != nil {
		return domain.TemplateSpec{}, fmt.Errorf("parse template file %s: %w", path, err)
	}
	return spec, nil
}

func parseTemplate(data []byte) (domain.TemplateSpec, error) {
	var spec domain.TemplateSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("unmarshal template: %w", err)
	}
	return spec, nil
}

// looksLikePath returns true when nameOrPath appears to be a filesystem path
// rather than a bare template name.
func looksLikePath(s string) bool {
	return strings.ContainsAny(s, "/\\") ||
		strings.HasSuffix(s, ".yml") ||
		strings.HasSuffix(s, ".yaml") ||
		s == "." || s == ".."
}

// extractTemplateRefs collects all unique variable references from a template
// body by scanning all string fields.
func extractTemplateRefs(t domain.TasktreeTemplate) []variable.VariableRef {
	var strs []string

	// Metadata fields
	strs = append(strs, t.Metadata.Name, t.Metadata.Description)
	for k, v := range t.Metadata.Annotations {
		strs = append(strs, k, v)
	}
	for k, v := range t.Metadata.Labels {
		strs = append(strs, k, v)
	}

	// Sources
	for _, src := range t.Spec.Sources {
		strs = append(strs, src.Name, string(src.Type), src.Path)
		if src.Git != nil {
			strs = append(strs, src.Git.URL, src.Git.Ref, src.Git.Branch)
		}
	}

	// Collect unique refs (by name) in order of first appearance.
	seen := make(map[string]struct{})
	var refs []variable.VariableRef
	for _, s := range strs {
		for _, ref := range variable.Parse(s) {
			if _, dup := seen[ref.Name]; !dup {
				seen[ref.Name] = struct{}{}
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

// suggest returns the closest declared parameter name to name, or "" if none
// is close enough (uses simple edit-distance approximation).
func suggest(name string, params map[string]struct{}) string {
	best := ""
	bestDist := len(name) + 1
	for p := range params {
		d := editDistance(name, p)
		if d < bestDist && d <= 2 {
			bestDist = d
			best = p
		}
	}
	return best
}

// editDistance computes the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	m, n := len(ra), len(rb)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if ra[i-1] == rb[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + min3(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
			}
		}
	}
	return dp[m][n]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
