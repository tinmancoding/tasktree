package repoalias

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
)

type File struct {
	Repos []Repo `yaml:"repos"`
}

type Repo struct {
	URL     string   `yaml:"url"`
	Aliases []string `yaml:"aliases"`
}

type Store struct {
	path string
}

func DefaultPath() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "tasktree", "repos.yml"), nil
	}
	if runtime.GOOS == "darwin" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home dir: %w", err)
		}
		return filepath.Join(homeDir, ".config", "tasktree", "repos.yml"), nil
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(userConfigDir, "tasktree", "repos.yml"), nil
}

func NewStore(path string) Store {
	return Store{path: path}
}

func NewDefaultStore() (Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return Store{}, err
	}
	return NewStore(path), nil
}

func (s Store) Path() string {
	return s.path
}

func (s Store) Load() (File, error) {
	var file File
	contents, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return file, fmt.Errorf("read repo aliases: %w", err)
	}
	if len(contents) == 0 {
		return File{}, nil
	}
	if err := yaml.Unmarshal(contents, &file); err != nil {
		return file, fmt.Errorf("parse repo aliases: %w", err)
	}
	file.normalize()
	return file, nil
}

func (s Store) Save(file File) error {
	file.normalize()
	contents, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode repo aliases: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create repo alias config dir: %w", err)
	}
	return fsx.AtomicWriteFile(s.path, contents, 0o644)
}

func (s Store) Resolve(name string) (string, bool, error) {
	file, err := s.Load()
	if err != nil {
		return "", false, err
	}
	for _, repo := range file.Repos {
		for _, alias := range repo.Aliases {
			if alias == name {
				return repo.URL, true, nil
			}
		}
	}
	return "", false, nil
}

func (f *File) normalize() {
	if f.Repos == nil {
		f.Repos = []Repo{}
	}
	byURL := make(map[string][]string, len(f.Repos))
	for _, repo := range f.Repos {
		if repo.URL == "" {
			continue
		}
		aliases := byURL[repo.URL]
		unique := make(map[string]struct{}, len(aliases)+len(repo.Aliases))
		for _, alias := range aliases {
			unique[alias] = struct{}{}
		}
		for _, alias := range repo.Aliases {
			if alias == "" {
				continue
			}
			if err := domain.ValidateRepoName(alias); err != nil {
				continue
			}
			if _, ok := unique[alias]; ok {
				continue
			}
			unique[alias] = struct{}{}
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		byURL[repo.URL] = aliases
	}
	cleaned := make([]Repo, 0, len(byURL))
	for url, aliases := range byURL {
		cleaned = append(cleaned, Repo{URL: url, Aliases: aliases})
	}
	sort.Slice(cleaned, func(i, j int) bool {
		return cleaned[i].URL < cleaned[j].URL
	})
	f.Repos = cleaned
}
