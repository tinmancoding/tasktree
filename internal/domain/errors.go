package domain

import "fmt"

type NotInTasktreeError struct {
	Start string
}

func (e NotInTasktreeError) Error() string {
	return "Not inside a tasktree (no .tasktree.toml found in current directory or parents).\nRun `tasktree init` to create one."
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
