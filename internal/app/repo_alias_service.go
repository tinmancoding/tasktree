package app

import (
	"fmt"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/repoalias"
)

type RepoAlias struct {
	Alias string
	URL   string
}

type RepoAliasRegistration struct {
	Alias  string
	Status string
	URL    string
}

type RepoAliasSetService struct {
	store repoalias.Store
}

func NewRepoAliasSetService(store repoalias.Store) RepoAliasSetService {
	return RepoAliasSetService{store: store}
}

func (s RepoAliasSetService) Run(alias, repoURL string) error {
	if err := domain.ValidateRepoName(alias); err != nil {
		return err
	}
	file, err := s.store.Load()
	if err != nil {
		return err
	}
	updated := make([]repoalias.Repo, 0, len(file.Repos)+1)
	foundURL := false
	for _, repo := range file.Repos {
		for _, existing := range repo.Aliases {
			if existing == alias && repo.URL != repoURL {
				return domain.RepoAliasInUseError{Alias: alias, URL: repo.URL}
			}
		}
		if repo.URL == repoURL {
			if !containsAlias(repo.Aliases, alias) {
				repo.Aliases = append(repo.Aliases, alias)
			}
			foundURL = true
		}
		updated = append(updated, repo)
	}
	if !foundURL {
		updated = append(updated, repoalias.Repo{URL: repoURL, Aliases: []string{alias}})
	}
	file.Repos = updated
	return s.store.Save(file)
}

func containsAlias(aliases []string, target string) bool {
	for _, alias := range aliases {
		if alias == target {
			return true
		}
	}
	return false
}

type RepoAliasRegisterDerivedService struct {
	store repoalias.Store
}

func NewRepoAliasRegisterDerivedService(store repoalias.Store) RepoAliasRegisterDerivedService {
	return RepoAliasRegisterDerivedService{store: store}
}

func (s RepoAliasRegisterDerivedService) Run(repoURL string) ([]RepoAliasRegistration, error) {
	aliases, err := domain.DeriveRepoAliases(repoURL)
	if err != nil {
		return nil, err
	}
	file, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	results := make([]RepoAliasRegistration, 0, len(aliases))
	repoIndex := -1
	for i, repo := range file.Repos {
		if repo.URL == repoURL {
			repoIndex = i
			break
		}
	}
	if repoIndex == -1 {
		file.Repos = append(file.Repos, repoalias.Repo{URL: repoURL, Aliases: []string{}})
		repoIndex = len(file.Repos) - 1
	}
	for _, alias := range aliases {
		ownerURL := ""
		for _, repo := range file.Repos {
			if containsAlias(repo.Aliases, alias) {
				ownerURL = repo.URL
				break
			}
		}
		switch {
		case ownerURL == repoURL:
			results = append(results, RepoAliasRegistration{Alias: alias, Status: "existing", URL: repoURL})
		case ownerURL != "":
			results = append(results, RepoAliasRegistration{Alias: alias, Status: "conflict", URL: ownerURL})
		default:
			file.Repos[repoIndex].Aliases = append(file.Repos[repoIndex].Aliases, alias)
			results = append(results, RepoAliasRegistration{Alias: alias, Status: "added", URL: repoURL})
		}
	}
	if err := s.store.Save(file); err != nil {
		return nil, err
	}
	return results, nil
}

type RepoAliasRemoveService struct {
	store repoalias.Store
}

func NewRepoAliasRemoveService(store repoalias.Store) RepoAliasRemoveService {
	return RepoAliasRemoveService{store: store}
}

func (s RepoAliasRemoveService) Run(alias string) error {
	file, err := s.store.Load()
	if err != nil {
		return err
	}
	updated := make([]repoalias.Repo, 0, len(file.Repos))
	removed := false
	for _, repo := range file.Repos {
		aliases := make([]string, 0, len(repo.Aliases))
		for _, existing := range repo.Aliases {
			if existing == alias {
				removed = true
				continue
			}
			aliases = append(aliases, existing)
		}
		repo.Aliases = aliases
		updated = append(updated, repo)
	}
	if !removed {
		return domain.RepoAliasNotFoundError{Alias: alias}
	}
	file.Repos = updated
	return s.store.Save(file)
}

type RepoAliasListService struct {
	store repoalias.Store
}

func NewRepoAliasListService(store repoalias.Store) RepoAliasListService {
	return RepoAliasListService{store: store}
}

func (s RepoAliasListService) Run() ([]RepoAlias, error) {
	file, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	aliases := make([]RepoAlias, 0)
	for _, repo := range file.Repos {
		for _, alias := range repo.Aliases {
			aliases = append(aliases, RepoAlias{Alias: alias, URL: repo.URL})
		}
	}
	return aliases, nil
}

type RepoAliasResolveService struct {
	store repoalias.Store
}

func NewRepoAliasResolveService(store repoalias.Store) RepoAliasResolveService {
	return RepoAliasResolveService{store: store}
}

func (s RepoAliasResolveService) Run(name string) (string, error) {
	resolved, ok, err := s.store.Resolve(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return name, nil
	}
	if resolved == "" {
		return "", fmt.Errorf("repository alias %q resolved to an empty URL", name)
	}
	return resolved, nil
}
