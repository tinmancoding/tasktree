package domain

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func DeriveRepoName(repoURL string) (string, error) {
	trimmed := strings.TrimSpace(repoURL)
	trimmed = strings.TrimSuffix(trimmed, "/")

	var base string
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" {
		base = path.Base(parsed.Path)
	} else {
		base = path.Base(trimmed)
		if strings.Contains(base, ":") {
			base = strings.Split(base, ":")[len(strings.Split(base, ":"))-1]
		}
	}

	base = strings.TrimSuffix(base, ".git")
	if err := ValidateRepoName(base); err != nil {
		return "", err
	}
	return base, nil
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
