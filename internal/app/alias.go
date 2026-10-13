package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/repoalias"
)

// Alias is one repository alias and the clone URL it names.
type Alias struct {
	Alias string `json:"alias"`
	URL   string `json:"url"`
}

func (a App) aliasStore() repoalias.Store { return repoalias.NewStore(a.Home.AliasesPath()) }

func validAliasName(s string) bool {
	return validRepoName(s) && !strings.ContainsAny(s, " \t\r\n")
}

// AddAlias registers alias → url (DN1, V1 semantics): the same pair is a
// no-op (changed=false); an alias that names another URL is a conflict.
func (a App) AddAlias(alias, url string) (changed bool, err error) {
	if !validAliasName(alias) {
		return false, ValidationError{fmt.Sprintf("invalid alias %q", alias)}
	}
	if strings.TrimSpace(url) == "" {
		return false, ValidationError{"alias URL must not be empty"}
	}
	err = a.aliasStore().Update(func(f *repoalias.File) error {
		found := false
		for i := range f.Repos {
			for _, existing := range f.Repos[i].Aliases {
				if existing == alias && f.Repos[i].URL != url {
					return hub.ConflictError{Message: fmt.Sprintf("alias %q is already used for %s", alias, f.Repos[i].URL)}
				}
			}
		}
		for i := range f.Repos {
			if f.Repos[i].URL != url {
				continue
			}
			found = true
			for _, existing := range f.Repos[i].Aliases {
				if existing == alias {
					return nil
				}
			}
			f.Repos[i].Aliases = append(f.Repos[i].Aliases, alias)
			changed = true
		}
		if !found {
			f.Repos = append(f.Repos, repoalias.Repo{URL: url, Aliases: []string{alias}})
			changed = true
		}
		return nil
	})
	return changed, err
}

// RemoveAlias drops an alias; an unknown alias is not found.
func (a App) RemoveAlias(alias string) error {
	return a.aliasStore().Update(func(f *repoalias.File) error {
		removed := false
		for i := range f.Repos {
			kept := f.Repos[i].Aliases[:0:0]
			for _, existing := range f.Repos[i].Aliases {
				if existing == alias {
					removed = true
					continue
				}
				kept = append(kept, existing)
			}
			f.Repos[i].Aliases = kept
		}
		if !removed {
			return NotFoundError{fmt.Sprintf("alias %q not found", alias)}
		}
		return nil
	})
}

// ListAliases returns every alias sorted by name. The result is never nil.
func (a App) ListAliases() ([]Alias, error) {
	f, err := a.aliasStore().Load()
	if err != nil {
		return nil, err
	}
	out := []Alias{}
	for _, r := range f.Repos {
		for _, name := range r.Aliases {
			out = append(out, Alias{Alias: name, URL: r.URL})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, nil
}
