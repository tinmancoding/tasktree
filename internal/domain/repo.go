package domain

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func repoURLPath(repoURL string) string {
	trimmed := strings.TrimSpace(repoURL)
	trimmed = strings.TrimSuffix(trimmed, "/")

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" {
		return parsed.Path
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 && !strings.Contains(parts[1], `\`) {
			return parts[1]
		}
	}
	return trimmed
}

func DeriveRepoName(repoURL string) (string, error) {
	base := path.Base(repoURLPath(repoURL))
	base = strings.TrimSuffix(base, ".git")
	if err := ValidateRepoName(base); err != nil {
		return "", err
	}
	return base, nil
}

func DeriveRepoAliases(repoURL string) ([]string, error) {
	repoName, err := DeriveRepoName(repoURL)
	if err != nil {
		return nil, err
	}
	aliases := []string{repoName}
	repoPath := strings.TrimSuffix(repoURLPath(repoURL), "/")
	owner := path.Base(path.Dir(repoPath))
	if owner != "" && owner != "." && owner != "/" {
		ownerRepo := fmt.Sprintf("%s-%s", owner, repoName)
		if ownerRepo != repoName {
			if err := ValidateRepoName(ownerRepo); err == nil {
				aliases = append(aliases, ownerRepo)
			}
		}
	}
	return aliases, nil
}

func ValidateRepoName(name string) error {
	if name == "" || name == "." || name == ".." {
		return InvalidRepoNameError{Name: name}
	}
	if filepath.Base(name) != name {
		return InvalidRepoNameError{Name: name}
	}
	if strings.Contains(name, string(filepath.Separator)) || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return InvalidRepoNameError{Name: name}
	}
	return nil
}

func RequestedCheckout(defaultBranch, requestedRef string) string {
	if requestedRef != "" {
		return requestedRef
	}
	return defaultBranch
}

func RepoPathForName(name string) string {
	return fmt.Sprintf("%s", name)
}
