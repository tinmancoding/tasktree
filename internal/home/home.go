package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/tinmancoding/tasktree/v2/internal/fsx"
	"gopkg.in/yaml.v3"
)

type JiraConfig struct {
	BaseURL string `yaml:"baseUrl,omitempty" json:"baseUrl"`
}
type Config struct {
	Remote    string     `yaml:"remote,omitempty" json:"remote"`
	TasksRoot string     `yaml:"tasksRoot" json:"tasksRoot"`
	Author    string     `yaml:"author,omitempty" json:"author"`
	Machine   string     `yaml:"machine" json:"machine"`
	Jira      JiraConfig `yaml:"jira,omitempty" json:"jira"`
}
type Checkout struct {
	Path  string `yaml:"path"`
	Focus string `yaml:"focus,omitempty"`
}
type LocalState struct {
	Checkouts map[string]Checkout `yaml:"checkouts"`
}

type Home struct{ Root string }

func Resolve() (Home, error) {
	if p := os.Getenv("TASKTREE_HOME"); p != "" {
		a, err := filepath.Abs(p)
		return Home{a}, err
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return Home{}, fmt.Errorf("resolve home: %w", err)
	}
	return Home{filepath.Join(u, ".tasktree")}, nil
}
func (h Home) ConfigPath() string         { return filepath.Join(h.Root, "config.yml") }
func (h Home) LocalPath() string          { return filepath.Join(h.Root, "local.yml") }
func (h Home) GitPath() string            { return filepath.Join(h.Root, "git") }
func (h Home) ClonesPath() string         { return filepath.Join(h.Root, "clones") }
func (h Home) ClonePath(id string) string { return filepath.Join(h.ClonesPath(), id+".git") }
func (h Home) AliasesPath() string        { return filepath.Join(h.Root, "aliases.yml") }
func Defaults() Config {
	u, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	if i := strings.IndexByte(host, '.'); i >= 0 {
		host = host[:i]
	}
	return Config{TasksRoot: filepath.Join(u, "Developer", "tasks"), Machine: host}
}
func (h Home) LoadConfig() (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(h.ConfigPath())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	if c.TasksRoot == "" {
		c.TasksRoot = Defaults().TasksRoot
	}
	if c.Machine == "" {
		c.Machine = Defaults().Machine
	}
	return c, nil
}
func (h Home) SaveConfig(c Config) error {
	if err := os.MkdirAll(h.Root, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return fsx.AtomicWriteFile(h.ConfigPath(), b, 0o644)
}
func (h Home) LoadLocal() (LocalState, error) {
	s := LocalState{Checkouts: map[string]Checkout{}}
	b, err := os.ReadFile(h.LocalPath())
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.Checkouts == nil {
		s.Checkouts = map[string]Checkout{}
	}
	return s, nil
}
func (h Home) saveLocalUnlocked(s LocalState) error {
	if s.Checkouts == nil {
		s.Checkouts = map[string]Checkout{}
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return fsx.AtomicWriteFile(h.LocalPath(), b, 0o644)
}

// UpdateLocal serializes cross-process read-modify-write operations and reloads
// after acquiring the lock so concurrent checkout entries cannot be lost.
func (h Home) UpdateLocal(update func(*LocalState) error) error {
	if err := os.MkdirAll(h.Root, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(h.LocalPath()+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	s, err := h.LoadLocal()
	if err != nil {
		return err
	}
	if err = update(&s); err != nil {
		return err
	}
	return h.saveLocalUnlocked(s)
}

func (h Home) SaveLocal(s LocalState) error {
	return h.UpdateLocal(func(current *LocalState) error { *current = s; return nil })
}
