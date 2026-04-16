package domain

import "fmt"

type NotInTasktreeError struct {
	Start string
}

func (e NotInTasktreeError) Error() string {
	return "Not inside a tasktree (no Tasktree.yml found in current directory or parents).\nRun `tasktree init` to create one."
}

type LegacyMetadataError struct {
	Path string
}

func (e LegacyMetadataError) Error() string {
	return fmt.Sprintf("Found legacy .tasktree.toml at %s.\nRun `tasktree migrate` to convert to Tasktree.yml.", e.Path)
}

type MetadataExistsError struct {
	Path string
}

func (e MetadataExistsError) Error() string {
	return fmt.Sprintf("tasktree metadata already exists at %s", e.Path)
}

type DuplicateRepoNameError struct {
	Name string
}

func (e DuplicateRepoNameError) Error() string {
	return fmt.Sprintf("repository %q already exists in this tasktree; use --name to choose a different checkout name", e.Name)
}

type DestinationExistsError struct {
	Path string
}

func (e DestinationExistsError) Error() string {
	return fmt.Sprintf("destination already exists at %s", e.Path)
}

type InvalidRepoNameError struct {
	Name string
}

func (e InvalidRepoNameError) Error() string {
	return fmt.Sprintf("invalid repository name %q", e.Name)
}

type UnresolvedRefError struct {
	RepoURL string
	Ref     string
}

func (e UnresolvedRefError) Error() string {
	return fmt.Sprintf("could not resolve ref %q for %s", e.Ref, e.RepoURL)
}

type BranchExistsError struct {
	Branch string
}

func (e BranchExistsError) Error() string {
	return fmt.Sprintf("branch %q already exists in this checkout", e.Branch)
}

type RepoNotFoundError struct {
	Name string
}

func (e RepoNotFoundError) Error() string {
	return fmt.Sprintf("repository %q was not found in this tasktree", e.Name)
}

type UnsafePathError struct {
	Path string
}

func (e UnsafePathError) Error() string {
	return fmt.Sprintf("unsafe path operation rejected for %s", e.Path)
}

type InvalidBranchNameError struct {
	Name string
}

func (e InvalidBranchNameError) Error() string {
	return fmt.Sprintf("invalid branch name %q", e.Name)
}

type RepoAliasNotFoundError struct {
	Alias string
}

func (e RepoAliasNotFoundError) Error() string {
	return fmt.Sprintf("repository alias %q was not found", e.Alias)
}

type RepoAliasInUseError struct {
	Alias string
	URL   string
}

func (e RepoAliasInUseError) Error() string {
	return fmt.Sprintf("repository alias %q is already used by %s", e.Alias, e.URL)
}

type UnknownSourceTypeError struct {
	Type SourceType
}

func (e UnknownSourceTypeError) Error() string {
	return fmt.Sprintf("unknown source type %q", e.Type)
}

type MissingSourceSpecError struct {
	Name string
	Type SourceType
}

func (e MissingSourceSpecError) Error() string {
	return fmt.Sprintf("source %q of type %q is missing its type-specific spec block", e.Name, e.Type)
}

// InvalidAnnotationKeyError is returned when an annotation key fails validation.
type InvalidAnnotationKeyError struct {
	Key    string
	Reason string
}

func (e InvalidAnnotationKeyError) Error() string {
	return fmt.Sprintf("invalid annotation key %q: %s", e.Key, e.Reason)
}
