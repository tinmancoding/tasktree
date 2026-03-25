# Tasktree Implementation Plan

## Overview

This plan turns `PRD.md` and `SYSTEM_DESIGN.md` into a concrete v1 execution sequence for building Tasktree as a Go CLI with `devbox shell` for local development.

The implementation goal is to ship a small, reliable CLI that:

- initializes a tasktree with `.tasktree.toml`
- adds standard Git checkouts from a local bare cache
- resets checkout `origin` to the original upstream URL after cloning from cache
- removes repositories from a tasktree without touching the cache
- lists configured repositories from metadata
- reports live Git status for repositories in the tasktree
- resolves tasktree context from any nested subdirectory

## Delivery Principles

- ship vertical slices early instead of building every abstraction up front
- keep Git behavior delegated to the system `git` binary
- prefer deterministic filesystem behavior over clever automation
- add tests alongside each major capability
- keep metadata writes atomic from the first implementation

## Assumptions

- Go is the implementation language
- local development runs through `devbox shell`
- Git is installed and available in the shell environment
- v1 is a local CLI only, with no daemon and no global configuration

## Suggested Milestones

### Milestone 0: Project bootstrap

Goal:

- establish the repo as a runnable Go CLI project with a reproducible local toolchain

Tasks:

- initialize `go.mod`
- add `devbox.json` with Go, Git, and lint tooling
- create `cmd/tasktree/main.go`
- scaffold CLI command registration
- add base package layout from `SYSTEM_DESIGN.md`
- add a simple `Makefile` or documented Go commands if desired

Acceptance criteria:

- `devbox shell` starts successfully
- `go test ./...` runs successfully
- `go run ./cmd/tasktree --help` prints command help

### Milestone 1: Core foundations

Goal:

- implement the shared primitives every command depends on

Tasks:

- implement filesystem helpers for safe path handling
- implement tasktree root upward resolution
- implement metadata structs and TOML encode/decode
- implement atomic file write helper for `.tasktree.toml`
- define typed application errors
- add unit tests for root resolution and metadata round trips

Acceptance criteria:

- code can locate the tasktree root from nested directories
- metadata can be loaded and saved without losing fields
- interrupted writes cannot leave a partially written `.tasktree.toml`

### Milestone 2: `tasktree init`

Goal:

- create a valid tasktree on disk

Tasks:

- implement `init` command wiring
- create target directory when needed
- infer tasktree name from directory name
- fail if `.tasktree.toml` already exists
- write initial metadata atomically
- add integration tests for current directory and explicit path usage

Acceptance criteria:

- `tasktree init` creates a valid `.tasktree.toml`
- `tasktree init <path>` creates the directory when missing
- repeated init fails with a clear error

### Milestone 3: Git wrapper and cache manager

Goal:

- make Git operations and cache lifecycle reliable before implementing `add`

Tasks:

- implement a `gitx` client wrapper around `os/exec`
- capture stdout, stderr, exit code, and command context in errors
- implement cache root resolution under the user cache directory
- implement deterministic cache path mapping with SHA-256 of repo URL
- implement bare cache create flow
- implement bare cache refresh flow
- add unit tests for cache mapping determinism
- add integration tests for bare cache creation and refresh using temp repos

Acceptance criteria:

- same repo URL always maps to the same cache path
- missing cache is created with `git clone --bare`
- existing cache is refreshed with `git fetch --all --prune`

### Milestone 4: `tasktree add`

Goal:

- add repositories into a tasktree with correct checkout and metadata behavior

Tasks:

- implement repo name derivation from repo URL
- validate `--name`, destination path conflicts, and duplicate metadata entries
- clone from the bare cache into the tasktree destination
- immediately reset `origin` to the original upstream repo URL
- resolve default branch when `--ref` is omitted
- support checkout by branch, tag, commit, or arbitrary ref
- support `--branch` creation from the resolved starting point
- collect resolved commit SHA and resolved ref
- persist metadata only after successful repo setup
- add rollback-safe behavior for failed clone or checkout steps
- add integration tests for all add resolution cases from the PRD

Acceptance criteria:

- `tasktree add <repo-url>` checks out the remote default branch
- `tasktree add <repo-url> --ref <x>` checks out the requested target
- `tasktree add <repo-url> --branch <y>` creates a local branch from default branch
- `tasktree add <repo-url> --ref <x> --branch <y>` creates a local branch from `x`
- `git remote get-url origin` in the checkout returns the original upstream URL, not the local cache path
- metadata reflects the requested and resolved state accurately

### Milestone 5: `tasktree list` and `tasktree root`

Goal:

- ship the simplest read-only inspection commands

Tasks:

- implement `list` using stored metadata only
- implement `root` using upward context resolution
- implement table output formatting helpers
- add integration tests for nested directory usage

Acceptance criteria:

- `tasktree list` prints configured repositories
- `tasktree root` prints the absolute tasktree root
- both commands work from nested directories inside a repo in the tasktree

### Milestone 6: `tasktree remove`

Goal:

- safely remove a repository checkout from the tasktree

Tasks:

- locate repo entry by logical name
- validate removal path is inside the tasktree root
- remove the checkout directory only
- remove the repo entry from metadata
- keep cache untouched
- add integration tests for remove success and not-found errors

Acceptance criteria:

- repository directory is deleted from the tasktree
- metadata entry is removed atomically
- cache directory still exists after removal

### Milestone 7: `tasktree status`

Goal:

- report live Git state for each repo in the tasktree

Tasks:

- inspect each checkout from metadata
- detect branch name or detached HEAD state
- detect dirty or clean state using Git
- render human-readable output similar to the PRD examples
- add integration tests for clean, modified, and detached states

Acceptance criteria:

- `status` reflects live repository state, not stale metadata
- detached HEAD from tag or commit checkout is shown clearly
- modified repositories are marked accordingly

### Milestone 8: Polish and hardening

Goal:

- tighten reliability, UX, and release readiness

Tasks:

- review and improve error messages across commands
- validate edge cases from the PRD error-handling section
- add best-effort locking for metadata and cache operations if included in v1
- document contributor workflow in `README.md`
- add release build instructions
- run final end-to-end manual verification flows

Acceptance criteria:

- all v1 commands work end to end from a clean checkout
- failure cases are clear and actionable
- local development steps are documented and reproducible

## Work Breakdown by Package

### `cmd/tasktree`

- process entrypoint
- command registration
- exit code handling

### `internal/cli`

- flags and argument validation
- command-to-service wiring
- user-facing text output and errors

### `internal/app`

- command orchestration
- validation sequencing
- coordination between metadata, git, cache, and filesystem services

### `internal/domain`

- tasktree and repo models
- domain validation rules
- typed errors

### `internal/gitx`

- Git command execution
- branch, ref, commit, and status helpers
- default branch discovery
- remote URL reset after cache clone

### `internal/cache`

- cache root resolution
- URL-to-cache-path mapping
- cache creation and refresh lifecycle

### `internal/metadata`

- TOML serialization
- atomic reads and writes
- repo entry update helpers

### `internal/fsx`

- safe path joins
- directory existence checks
- delete and atomic write helpers

### `internal/output`

- table formatting for `list` and `status`
- shared text rendering helpers

## Test Plan

### Unit tests

- tasktree root upward search
- repo name derivation from Git URLs
- cache hash mapping
- metadata parse and serialize round trips
- path safety and containment checks
- atomic write helper behavior where practical

### Integration tests

- `init` in current directory and explicit path
- `add` with default branch, explicit branch, tag, commit, and `--branch`
- clone-from-cache plus `origin` reset to upstream URL
- duplicate names and existing destination errors
- `list` output from metadata
- `root` from nested repo subdirectories
- `remove` updates metadata and leaves cache intact
- `status` shows clean, modified, and detached states

### Manual verification checklist

- create a tasktree from scratch
- add two repos with matching feature branches
- enter a nested subdirectory and run `tasktree root`
- modify a file and confirm `tasktree status`
- run `git pull` or `git remote -v` inside the checkout to verify upstream `origin`

## Dependency Recommendations

Recommended external dependencies:

- `cobra` for CLI structure
- a TOML library such as `go-toml/v2` or `BurntSushi/toml`

Recommended selection criteria:

- stable and widely used
- minimal API surface
- easy to test

Keep the dependency set small. Git interaction should remain through `os/exec`, not a Git library.

## Risks and Mitigations

### Remote default branch resolution is inconsistent

Mitigation:

- standardize the logic inside `gitx`
- cover it with integration tests against temp repos

### Metadata gets out of sync with disk state

Mitigation:

- persist metadata only after successful repo setup
- compute dynamic state live in `status`
- keep `remove` path validation strict

### Cache clone leaves wrong remote configured

Mitigation:

- always run `git remote set-url origin <repo-url>` after cloning from cache
- test `git remote get-url origin` in integration tests

### Concurrent operations race on metadata or cache

Mitigation:

- use atomic file replacement
- add best-effort lock files around metadata and cache updates

### Overengineering slows v1 delivery

Mitigation:

- implement milestone by milestone
- avoid future-only abstractions unless they directly simplify current code

## Definition of Done for v1

Tasktree v1 is done when:

- all six core commands are implemented
- `.tasktree.toml` is written atomically and matches the v1 schema
- repository checkouts are cloned through the local bare cache
- checkout `origin` points to the original upstream repository URL
- commands work from the tasktree root and nested subdirectories
- automated tests cover the main happy paths and key edge cases
- a contributor can use `devbox shell` to run tests and the CLI locally

## Recommended Build Order

If implementation begins immediately, use this order:

1. bootstrap project and Devbox
2. metadata plus root resolution
3. `init`
4. Git wrapper plus cache manager
5. `add`
6. `list` and `root`
7. `remove`
8. `status`
9. polish, tests, and docs

This sequence delivers user-visible value early while keeping the riskiest Git behavior isolated before the rest of the command surface depends on it.
