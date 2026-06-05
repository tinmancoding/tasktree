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

// TemplateNotFoundError is returned when a named template cannot be found
// in any of the template discovery paths.
type TemplateNotFoundError struct {
	Name string
}

func (e TemplateNotFoundError) Error() string {
	return fmt.Sprintf("template %q not found in search paths", e.Name)
}

// MissingVariableError is returned when a required template variable has no
// value after exhausting all resolution sources.
type MissingVariableError struct {
	Name string
}

func (e MissingVariableError) Error() string {
	return fmt.Sprintf("missing required variable %q", e.Name)
}

// UnknownVariableError is returned when a template body references a variable
// that was not declared in the template's parameters section.
type UnknownVariableError struct {
	Name       string
	Suggestion string // non-empty when a close match exists
}

func (e UnknownVariableError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("template references unknown variable %q (did you mean %q?)", e.Name, e.Suggestion)
	}
	return fmt.Sprintf("template references unknown variable %q", e.Name)
}

// InvalidVariableNameError is returned when a variable name in a template does
// not conform to the required pattern [a-z][a-z0-9_]*.
type InvalidVariableNameError struct {
	Name string
}

func (e InvalidVariableNameError) Error() string {
	return fmt.Sprintf("invalid variable name %q (must match [a-z][a-z0-9_]*)", e.Name)
}

// DuplicateSourceNameError is returned when a source name or path conflicts
// with an existing entry in Tasktree.yml.
type DuplicateSourceNameError struct {
	Name string
}

func (e DuplicateSourceNameError) Error() string {
	return fmt.Sprintf("source %q already exists in this tasktree; use --name to choose a different name", e.Name)
}

// InvalidSourceNameError is returned when a source name fails validation.
type InvalidSourceNameError struct {
	Name string
}

func (e InvalidSourceNameError) Error() string {
	return fmt.Sprintf("invalid source name %q", e.Name)
}

// InvalidHTTPSSchemeError is returned when an http/archive source URL does not
// use the HTTPS scheme.
type InvalidHTTPSSchemeError struct {
	URL string
}

func (e InvalidHTTPSSchemeError) Error() string {
	return fmt.Sprintf("URL %q must use the https:// scheme", e.URL)
}

// SHA256MismatchError is returned when a downloaded file's digest does not
// match the expected sha256 value declared in the source spec.
type SHA256MismatchError struct {
	URL      string
	Expected string
	Got      string
}

func (e SHA256MismatchError) Error() string {
	return fmt.Sprintf("sha256 mismatch for %s: expected %s, got %s", e.URL, e.Expected, e.Got)
}

// UnknownArchiveFormatError is returned when the archive format cannot be
// inferred from the URL and was not specified explicitly.
type UnknownArchiveFormatError struct {
	URL string
}

func (e UnknownArchiveFormatError) Error() string {
	return fmt.Sprintf("cannot determine archive format for %q; set the 'format' field explicitly", e.URL)
}

// LocalSourceNotFoundError is returned when the sourcePath for a local source
// does not exist on disk.
type LocalSourceNotFoundError struct {
	Path string
}

func (e LocalSourceNotFoundError) Error() string {
	return fmt.Sprintf("local source path %q does not exist", e.Path)
}

// EmptyBootstrapFieldError is returned when a required bootstrap step field
// (name or run) is empty.
type EmptyBootstrapFieldError struct {
	Index int
	Field string
}

func (e EmptyBootstrapFieldError) Error() string {
	return fmt.Sprintf("bootstrap step #%d: %q must not be empty", e.Index+1, e.Field)
}

// DuplicateBootstrapNameError is returned when two bootstrap steps share the
// same name.
type DuplicateBootstrapNameError struct {
	Name string
}

func (e DuplicateBootstrapNameError) Error() string {
	return fmt.Sprintf("duplicate bootstrap step name %q", e.Name)
}

// WorkdirEscapesRootError is returned when a bootstrap step's workdir resolves
// outside the workspace root.
type WorkdirEscapesRootError struct {
	Name    string
	Workdir string
}

func (e WorkdirEscapesRootError) Error() string {
	return fmt.Sprintf("bootstrap step %q: workdir %q escapes the workspace root", e.Name, e.Workdir)
}

// WorkdirNotFoundError is returned when a bootstrap step's workdir does not
// exist (or is not a directory) at apply time.
type WorkdirNotFoundError struct {
	Name        string
	Workdir     string
	ResolvedDir string
}

func (e WorkdirNotFoundError) Error() string {
	if e.Workdir == "" {
		return fmt.Sprintf("bootstrap step %q: workspace root %q does not exist", e.Name, e.ResolvedDir)
	}
	return fmt.Sprintf("bootstrap step %q: workdir %q does not exist", e.Name, e.Workdir)
}

// StepFailedError is returned when a bootstrap step exits non-zero. The child
// exit code is preserved for the engine to map to run failure.
type StepFailedError struct {
	Name     string
	Workdir  string
	ExitCode int
}

func (e StepFailedError) Error() string {
	loc := "."
	if e.Workdir != "" {
		loc = e.Workdir
	}
	return fmt.Sprintf("bootstrap step %q failed (exit %d) in %s; apply aborted. Fix and re-run 'tasktree apply'.", e.Name, e.ExitCode, loc)
}
