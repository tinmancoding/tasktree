// Package template provides V2 template discovery and validation.
package template

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
	"github.com/tinmancoding/tasktree/v2/internal/variable"
	"gopkg.in/yaml.v3"
)

//go:embed builtins/*.yml
var builtins embed.FS

type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}
type Parameter struct {
	Name        string `yaml:"name"`
	Required    bool   `yaml:"required,omitempty"`
	Default     string `yaml:"default,omitempty"`
	Description string `yaml:"description,omitempty"`
}
type Task struct {
	Title       string   `yaml:"title,omitempty"`
	Handle      string   `yaml:"handle,omitempty"`
	Jira        string   `yaml:"jira,omitempty"`
	Status      string   `yaml:"status,omitempty"`
	Labels      []string `yaml:"labels,omitempty"`
	Description string   `yaml:"description,omitempty"`
}
type Repo struct {
	Repo   string `yaml:"repo"`
	Name   string `yaml:"name,omitempty"`
	Branch string `yaml:"branch,omitempty"`
	From   string `yaml:"from,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}
type File struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode,omitempty"`
}
type Symlink struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target"`
}
type Tasktree struct {
	Repos     []Repo           `yaml:"repos,omitempty"`
	Symlinks  []Symlink        `yaml:"symlinks,omitempty"`
	Files     []File           `yaml:"files,omitempty"`
	Bootstrap []bootstrap.Step `yaml:"bootstrap,omitempty"`
}
type Spec struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Parameters []Parameter `yaml:"parameters,omitempty"`
	Task       Task        `yaml:"task"`
	Tasktree   *Tasktree   `yaml:"tasktree,omitempty"`
	Raw        []byte      `yaml:"-"`
	Source     string      `yaml:"-"`
}
type Store struct{ UserDir string }

func New(userDir string) Store { return Store{UserDir: userDir} }
func parse(b []byte, source string) (Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, err
	}
	s.Raw = append([]byte(nil), b...)
	s.Source = source
	return s, nil
}
func (s Store) List() ([]Spec, error) {
	by := map[string]Spec{}
	if entries, err := os.ReadDir(s.UserDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml")) {
				continue
			}
			b, x := os.ReadFile(filepath.Join(s.UserDir, e.Name()))
			if x != nil {
				return nil, x
			}
			v, x := parse(b, "user")
			if x != nil {
				return nil, fmt.Errorf("load template %s: %w", e.Name(), x)
			}
			by[v.Metadata.Name] = v
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	entries, _ := builtins.ReadDir("builtins")
	for _, e := range entries {
		b, _ := builtins.ReadFile("builtins/" + e.Name())
		v, x := parse(b, "builtin")
		if x != nil {
			return nil, x
		}
		if _, ok := by[v.Metadata.Name]; !ok {
			by[v.Metadata.Name] = v
		}
	}
	out := make([]Spec, 0, len(by))
	for _, v := range by {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metadata.Name < out[j].Metadata.Name })
	return out, nil
}
func (s Store) Load(nameOrPath string) (Spec, error) {
	if strings.ContainsAny(nameOrPath, "/\\") || strings.HasSuffix(nameOrPath, ".yml") || strings.HasSuffix(nameOrPath, ".yaml") {
		b, e := os.ReadFile(nameOrPath)
		if e != nil {
			return Spec{}, e
		}
		return parse(b, "file")
	}
	all, e := s.List()
	if e != nil {
		return Spec{}, e
	}
	for _, v := range all {
		if v.Metadata.Name == nameOrPath {
			return v, nil
		}
	}
	return Spec{}, fmt.Errorf("template %q not found", nameOrPath)
}
func stringsOf(s Spec) []string {
	x := []string{s.Task.Title, s.Task.Handle, s.Task.Jira, s.Task.Status, s.Task.Description}
	x = append(x, s.Task.Labels...)
	if s.Tasktree != nil {
		for _, r := range s.Tasktree.Repos {
			x = append(x, r.Repo, r.Name, r.Branch, r.From, r.Mode)
		}
		for _, f := range s.Tasktree.Files {
			x = append(x, f.Path, f.Content, f.Mode)
		}
		for _, l := range s.Tasktree.Symlinks {
			x = append(x, l.Path, l.Target)
		}
		for _, b := range s.Tasktree.Bootstrap {
			x = append(x, b.Name, b.Run, b.Workdir)
			for k, v := range b.Env {
				x = append(x, k, v)
			}
		}
	}
	return x
}
func Refs(s Spec) []variable.Ref {
	var out []variable.Ref
	for _, x := range stringsOf(s) {
		out = append(out, variable.Parse(x)...)
	}
	return out
}
func Validate(s Spec) error {
	if s.APIVersion != "tasktree.dev/v2" {
		return fmt.Errorf("invalid apiVersion %q", s.APIVersion)
	}
	if s.Kind != "Template" {
		return fmt.Errorf("invalid kind %q: expected Template", s.Kind)
	}
	if s.Metadata.Name == "" {
		return fmt.Errorf("metadata.name must not be empty")
	}
	decl := map[string]bool{}
	for _, p := range s.Parameters {
		if err := variable.ValidateName(p.Name); err != nil {
			return err
		}
		if decl[p.Name] {
			return fmt.Errorf("duplicate parameter %q", p.Name)
		}
		decl[p.Name] = true
	}
	for _, r := range Refs(s) {
		if !decl[r.Name] {
			return fmt.Errorf("variable %q is not declared in parameters", r.Name)
		}
	}
	return nil
}
