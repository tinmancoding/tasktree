package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
	"gopkg.in/yaml.v3"
)

// BootstrapStep is the JSON row of `bootstrap list --json`.
type BootstrapStep struct {
	Name    string            `json:"name"`
	Run     string            `json:"run"`
	Workdir string            `json:"workdir"`
	Env     map[string]string `json:"env"`
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ParseEnvAssignments turns repeated K=V flags into a map, validating keys.
func ParseEnvAssignments(pairs []string) (map[string]string, error) {
	env := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || !envKeyPattern.MatchString(k) {
			return nil, ValidationError{fmt.Sprintf("invalid --env %q (expected KEY=VALUE with KEY matching [A-Za-z_][A-Za-z0-9_]*)", p)}
		}
		env[k] = v
	}
	return env, nil
}

// ValidateBootstrapStep checks a step being added: a name, a command, a
// relative workdir that stays inside the root, and valid env keys.
func ValidateBootstrapStep(s bootstrap.Step) error {
	if strings.TrimSpace(s.Name) == "" || strings.ContainsAny(s.Name, "\r\n") {
		return ValidationError{"bootstrap step name must be a non-empty single line"}
	}
	if strings.TrimSpace(s.Run) == "" {
		return ValidationError{"bootstrap step run command must not be empty"}
	}
	if s.Workdir != "" {
		clean := filepath.Clean(s.Workdir)
		escapes := clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
		if filepath.IsAbs(s.Workdir) || escapes {
			return ValidationError{fmt.Sprintf("bootstrap workdir %q must be relative and inside the tasktree root", s.Workdir)}
		}
	}
	for k := range s.Env {
		if !envKeyPattern.MatchString(k) {
			return ValidationError{fmt.Sprintf("invalid environment variable name %q", k)}
		}
	}
	return nil
}

func toBootstrapRows(steps []bootstrap.Step) []BootstrapStep {
	rows := make([]BootstrapStep, 0, len(steps))
	for _, s := range steps {
		env := map[string]string{}
		for k, v := range s.Env {
			env[k] = v
		}
		rows = append(rows, BootstrapStep{Name: s.Name, Run: s.Run, Workdir: s.Workdir, Env: env})
	}
	return rows
}

func addStep(steps []bootstrap.Step, step bootstrap.Step) ([]bootstrap.Step, error) {
	if err := ValidateBootstrapStep(step); err != nil {
		return nil, err
	}
	for _, existing := range steps {
		if existing.Name == step.Name {
			return nil, ValidationError{fmt.Sprintf("bootstrap step %q already exists", step.Name)}
		}
	}
	if len(step.Env) == 0 {
		step.Env = nil
	}
	return append(append([]bootstrap.Step(nil), steps...), step), nil
}

func removeStep(steps []bootstrap.Step, name string) ([]bootstrap.Step, error) {
	out := make([]bootstrap.Step, 0, len(steps))
	found := false
	for _, s := range steps {
		if s.Name == name {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		return nil, NotFoundError{fmt.Sprintf("bootstrap step %q not found", name)}
	}
	return out, nil
}

func readRootConfig(root string) (TasktreeConfig, error) {
	var cfg TasktreeConfig
	b, err := os.ReadFile(filepath.Join(root, ".tasktree", "config.yml"))
	if err != nil {
		return cfg, err
	}
	return cfg, yaml.Unmarshal(b, &cfg)
}

func (a App) writeRootConfig(root string, cfg TasktreeConfig) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".tasktree", "config.yml"), b, 0o644)
}

// definitionConfig reads .tasktree/config.yml from the task branch without a
// checkout. A task without a definition yields a fresh config.
func (a App) definitionConfig(ctx context.Context, id string) (TasktreeConfig, bool, error) {
	cfg := TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: id}
	out, _, err := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "show", "refs/heads/task/"+id+":.tasktree/config.yml")
	if err != nil {
		return cfg, false, nil
	}
	var read TasktreeConfig
	if err = yaml.Unmarshal([]byte(out), &read); err != nil {
		return cfg, true, fmt.Errorf("parse .tasktree/config.yml on task/%s: %w", id, err)
	}
	return read, true, nil
}

// BootstrapSteps lists the configured steps of a checkout (root != "") or of
// a definition without a checkout.
func (a App) BootstrapSteps(ctx context.Context, id, root string) ([]BootstrapStep, error) {
	var cfg TasktreeConfig
	var err error
	if root != "" {
		if err = validateRootIdentity(id, root); err != nil {
			return nil, err
		}
		cfg, err = readRootConfig(root)
	} else {
		cfg, _, err = a.definitionConfig(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return toBootstrapRows(cfg.Bootstrap), nil
}

// BootstrapAdd appends a step and records the change: a structural commit on
// the checkout, or a plumbing write to the task branch (D47).
func (a App) BootstrapAdd(ctx context.Context, id, root string, step bootstrap.Step) error {
	return a.editBootstrap(ctx, id, root, "bootstrap add "+step.Name, func(steps []bootstrap.Step) ([]bootstrap.Step, error) { return addStep(steps, step) })
}

// BootstrapRemove deletes a step by name.
func (a App) BootstrapRemove(ctx context.Context, id, root, name string) error {
	return a.editBootstrap(ctx, id, root, "bootstrap rm "+name, func(steps []bootstrap.Step) ([]bootstrap.Step, error) { return removeStep(steps, name) })
}

func (a App) editBootstrap(ctx context.Context, id, root, summary string, edit func([]bootstrap.Step) ([]bootstrap.Step, error)) error {
	message := fmt.Sprintf("tasktree(%s): %s", id, summary)
	if root != "" {
		if err := validateRootIdentity(id, root); err != nil {
			return err
		}
		cfg, err := readRootConfig(root)
		if err != nil {
			return err
		}
		if cfg.Bootstrap, err = edit(cfg.Bootstrap); err != nil {
			return err
		}
		if err = a.writeRootConfig(root, cfg); err != nil {
			return err
		}
		return a.commitRoot(ctx, id, root, message, ".tasktree/config.yml")
	}
	return a.editDefinition(ctx, id, message, func(ms []module, f map[string]string) ([]module, map[string]string, error) {
		cfg, _, err := a.definitionConfig(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if cfg.Bootstrap, err = edit(cfg.Bootstrap); err != nil {
			return nil, nil, err
		}
		b, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, nil, err
		}
		f[".tasktree/config.yml"] = string(b)
		return ms, f, nil
	})
}
