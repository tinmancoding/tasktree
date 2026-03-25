# Tasktree System Design

## Overview

Tasktree is a task-first CLI for creating and managing isolated local workspaces that contain one or more standard Git checkouts.

This design targets:

- Go as the implementation language
- `devbox shell` as the primary local development environment
- normal Git clones, not Git worktrees
- zero required global configuration for v1

The core design principle is simple: a tasktree is just a directory with a `.tasktree.toml` file plus repository subdirectories. All command behavior flows from that filesystem model.

## Goals

The system design must support the PRD requirements for:

- `tasktree init`
- `tasktree add`
- `tasktree remove`
- `tasktree list`
- `tasktree status`
- `tasktree root`
- upward context detection from any nested directory
- deterministic bare-repo caching under the user cache directory
- atomic metadata updates
- safe, debuggable behavior with Git as the source of truth

## Non-Goals

This design intentionally excludes:

- Git worktree integration
- background daemons
- global manifests or repository catalogs
- interactive TUI flows
- long-running cache managers
- distributed coordination across tasktrees

## High-Level Architecture

The system is a single local CLI process with five main layers:

1. CLI layer for argument parsing and user-facing output
2. Application layer for command orchestration and validation
3. Domain layer for tasktree, repository, and cache rules
4. Infrastructure layer for filesystem, Git, and metadata persistence
5. Platform layer provided by the OS, installed Git, and local cache directories

```text
+---------------------------+
| CLI Commands              |
| init add remove list ...  |
+------------+--------------+
             |
             v
+---------------------------+
| Application Services      |
| Init Add Remove Status    |
+------------+--------------+
             |
   +---------+---------+
   v                   v
+----------+      +----------------+
| Metadata  |      | Git Operations |
| TOML I/O  |      | via git CLI    |
+----------+      +----------------+
   |                   |
   v                   v
+-----------------------------------+
| Filesystem + Cache + Repo Clones  |
+-----------------------------------+
```

## Why Go

Go is a good fit for v1 because it provides:

- easy single-binary distribution
- strong standard library support for filesystem and process execution
- simple cross-platform path handling
- low operational complexity
- fast startup for CLI usage

The design favors calling the system `git` binary through `os/exec` instead of embedding Git behavior in-process. That keeps Tasktree aligned with real Git semantics and reduces the risk of mismatches around refs, remote HEAD resolution, checkout behavior, and dirty state detection.

## Why `git` CLI Instead of a Go Git Library

Tasktree should use the installed `git` binary as the source of truth for repository operations.

Reasons:

- the PRD explicitly treats Git as the source of truth
- branch and ref resolution behavior should match normal developer workflows
- remote default branch resolution is simpler and more faithful with native Git
- status and detached HEAD behavior are easier to surface accurately
- lower semantic drift than a partial reimplementation

Tradeoff:

- Tasktree depends on `git` being installed locally

This is acceptable because the product is explicitly a local developer tool.

## Runtime Model

Each invocation of `tasktree` is stateless at the process level.

Persistent state exists only in:

- the tasktree directory itself
- `.tasktree.toml`
- cached bare repositories under the user cache directory
- the Git repositories cloned into the tasktree

There is no daemon, lock server, or background refresh process in v1.

## Filesystem Layout

Example tasktree:

```text
~/ws/feature-payments/
  .tasktree.toml
  api/
  web/
  contracts/
```

Example cache layout:

```text
~/.cache/tasktree/
  repos/
    8f3d2c0a6f4d.../
      config
      HEAD
      objects/
      refs/
```

The cache directory naming scheme should be deterministic and safe for all URL formats. A hashed mapping is preferred for v1.

## Metadata Design

Tasktree metadata lives in `.tasktree.toml` at the tasktree root.

### Design rules

- human-readable and easy to inspect
- stores requested and resolved repository setup state
- avoids storing highly dynamic Git status
- written atomically
- schema-versioned from day one

### Proposed v1 schema

```toml
version = 1
name = "feature-payments"
created_at = "2026-03-25T12:00:00Z"

[[repos]]
name = "api"
path = "api"
url = "git@github.com:myorg/api.git"
checkout = "main"
resolved_ref = "refs/heads/main"
commit = "abc123def4567890"
branch = "feature/payments"
```

### In-memory representation

```go
type TasktreeFile struct {
	Version   int        `toml:"version"`
	Name      string     `toml:"name"`
	CreatedAt time.Time  `toml:"created_at"`
	Repos     []RepoSpec `toml:"repos"`
}

type RepoSpec struct {
	Name        string `toml:"name"`
	Path        string `toml:"path"`
	URL         string `toml:"url"`
	Checkout    string `toml:"checkout"`
	ResolvedRef string `toml:"resolved_ref"`
	Commit      string `toml:"commit"`
	Branch      string `toml:"branch,omitempty"`
}
```

`checkout` stores the user-facing requested target. `resolved_ref` and `commit` store what Git actually resolved during `add`.

## Package Structure

A simple package layout for v1:

```text
cmd/tasktree/
  main.go

internal/cli/
  root.go
  init.go
  add.go
  remove.go
  list.go
  status.go
  rootcmd.go

internal/app/
  init_service.go
  add_service.go
  remove_service.go
  list_service.go
  status_service.go
  root_service.go

internal/domain/
  tasktree.go
  repo.go
  errors.go

internal/gitx/
  client.go
  refs.go
  status.go

internal/metadata/
  store.go
  atomic.go

internal/cache/
  mapper.go
  manager.go

internal/fsx/
  paths.go
  dirs.go
  atomic.go

internal/output/
  table.go
  text.go
```

Notes:

- `internal` keeps implementation details private
- `gitx` wraps all `git` command execution
- `app` orchestrates use cases without owning low-level Git or filesystem details
- `metadata` owns TOML serialization and atomic persistence

## CLI Framework

Two good options exist:

- standard library `flag`
- `cobra`

Recommended choice for v1: `cobra`

Reasons:

- natural fit for subcommands like `init`, `add`, `remove`, `list`, `status`, `root`
- consistent help generation
- clean flag handling for future command growth
- widely understood in Go CLI projects

If dependency minimization is prioritized above ergonomics, `flag` is still viable. The architecture does not depend on either choice.

## Local Development With Devbox

Local development should run through `devbox shell` so contributors get a reproducible toolchain.

Recommended `devbox.json` contents:

```json
{
  "packages": [
    "go@latest",
    "git@latest",
    "golangci-lint@latest"
  ]
}
```

Recommended contributor workflow:

```bash
devbox shell
go test ./...
go run ./cmd/tasktree --help
```

Devbox is a development convenience only. The shipped `tasktree` binary must not depend on Devbox at runtime.

## Core Services

### 1. Context Resolution Service

Responsibility:

- find the current tasktree root by walking upward from a starting directory

Algorithm:

1. start from `cwd`
2. check for `.tasktree.toml`
3. if found, return that directory
4. otherwise move to parent
5. stop at filesystem root and return a typed error

This service is shared by `add`, `remove`, `list`, `status`, and `root`.

### 2. Metadata Store

Responsibility:

- load `.tasktree.toml`
- validate schema basics
- update repo entries
- write atomically

Atomic write approach:

1. serialize TOML to bytes
2. write to a temp file in the same directory
3. `fsync` temp file if supported
4. rename temp file over `.tasktree.toml`
5. optionally `fsync` parent directory on platforms where this matters

This avoids partial metadata corruption during interruption.

### 3. Cache Manager

Responsibility:

- map repo URL to deterministic cache path
- initialize bare cache when missing
- refresh cache on demand before clone

Recommended mapping:

- normalize the exact input URL string
- hash using SHA-256
- use the hex digest as the directory name

Example:

```text
cache root: ~/.cache/tasktree/repos/
repo url:   git@github.com:myorg/api.git
cache dir:  ~/.cache/tasktree/repos/8f3d2c0a6f4d...
```

Why hashing:

- deterministic
- safe across URL characters
- avoids path length and escaping issues
- easy to implement consistently across platforms

### 4. Git Client

Responsibility:

- wrap all Git command execution
- return structured errors with stderr context
- keep command behavior centralized and testable

Representative operations:

- `git clone --bare <url> <cachePath>`
- `git -C <cachePath> fetch --all --prune`
- `git clone <cachePath> <destPath>`
- `git -C <repoPath> checkout <ref>`
- `git -C <repoPath> checkout -b <branch> <startPoint>`
- `git -C <repoPath> rev-parse HEAD`
- `git -C <repoPath> symbolic-ref --quiet --short HEAD`
- `git -C <repoPath> status --porcelain`

The wrapper should capture stdout, stderr, exit code, and the command arguments used.

### 5. Output Formatter

Responsibility:

- produce stable human-readable text tables for `list` and `status`
- keep formatting separate from business logic

JSON output is not required for v1, but the formatter abstraction makes that easy to add later.

## Command Designs

### `tasktree init [path]`

Flow:

1. resolve target path
2. create directory if needed
3. fail if `.tasktree.toml` already exists
4. build initial metadata with `version`, inferred `name`, and `created_at`
5. write metadata atomically

Inferred name rule:

- use the base directory name of the tasktree root

### `tasktree add <repo-url>`

Flow:

1. resolve current tasktree root
2. load metadata
3. derive checkout name from URL unless `--name` is provided
4. validate name/path conflicts against metadata and filesystem
5. resolve deterministic cache path
6. create cache if missing, otherwise refresh it
7. clone from cache into destination directory
8. reset checkout `origin` to the original repository URL
9. resolve target checkout:
   - `--ref` if provided
   - remote default branch otherwise
10. check out target
11. if `--branch` is provided, create and switch to local branch
12. determine resolved ref and commit SHA
13. append repo entry to metadata
14. atomically persist metadata

Important sequencing rule:

- do not write metadata until repository setup succeeds

This satisfies the PRD requirement to avoid corrupt or misleading metadata after partial failures.

### Default Branch Resolution

Recommended approach:

1. clone from the bare cache into the destination
2. inspect `refs/remotes/origin/HEAD` or use Git symbolic-ref commands in the fresh checkout
3. resolve the default branch name from origin HEAD
4. check it out explicitly if needed

Alternative implementations may query the bare cache directly before cloning, but using the checkout keeps the logic straightforward and consistent.

### `tasktree remove <name>`

Flow:

1. resolve tasktree root
2. load metadata
3. locate repo entry by `name`
4. remove the checkout directory from the tasktree root
5. remove the metadata entry
6. atomically persist metadata

Safety rules:

- only remove directories that are registered in metadata and live inside the tasktree root
- do not touch the cache
- fail clearly if the entry does not exist

### `tasktree list`

Flow:

1. resolve tasktree root
2. load metadata
3. render a table from stored repo metadata

This command reads metadata only and does not consult live Git state.

### `tasktree status`

Flow:

1. resolve tasktree root
2. load metadata
3. for each repo entry, inspect the live checkout
4. collect branch or detached HEAD state
5. collect dirty or clean state from Git
6. render output

This command intentionally computes dynamic state live instead of persisting it.

### `tasktree root`

Flow:

1. resolve tasktree root from `cwd`
2. print the absolute path

## Detailed Git Behavior

### Cache Create or Refresh

If cache does not exist:

```bash
git clone --bare <repo-url> <cache-path>
```

If cache exists:

```bash
git -C <cache-path> fetch --all --prune
```

### Clone From Cache

Recommended command:

```bash
git clone <cache-path> <dest-path>
```

This creates a normal checkout that is independent from other tasktrees.

Immediately after cloning from the local bare cache, Tasktree should reset the checkout's `origin` remote to the original repository URL.

Recommended follow-up command:

```bash
git -C <dest-path> remote set-url origin <repo-url>
```

Why this matters:

- users can run normal `git pull` and `git push` inside the checkout without extra setup
- the local cache remains only a clone acceleration mechanism, not the long-term remote of record
- checkout behavior matches user expectations for a normal repository clone

### Checkout Rules

Case handling should match the PRD:

- no `--ref`, no `--branch`: use remote default branch
- `--ref X`, no `--branch`: check out `X`; detached HEAD is allowed
- `--ref X --branch Y`: check out `X`, create branch `Y`, switch to `Y`
- no `--ref`, `--branch Y`: resolve default branch, create branch `Y`, switch to `Y`

### Capturing Resolved State

After successful checkout, collect:

- `commit`: `git rev-parse HEAD`
- `resolved_ref`: best-effort full symbolic ref if one exists
- current branch name if a new local branch was created

For detached checkouts:

- `resolved_ref` may be a tag ref or another resolved Git ref when available
- if only a commit is resolvable, `commit` remains the canonical stored truth

## Error Model

Errors should be typed internally and rendered with clear CLI messages.

Suggested categories:

- not in tasktree
- metadata already exists
- metadata parse failure
- duplicate repo name
- destination already exists
- invalid or unresolved ref
- branch creation conflict
- cache refresh failure
- Git command failure
- unsafe path operation

Example user-facing error:

```text
Not inside a tasktree (no .tasktree.toml found in current directory or parents).
Run `tasktree init` to create one.
```

## Concurrency and Locking

V1 can remain single-process in practice, but two local concerns exist:

- concurrent writes to `.tasktree.toml`
- concurrent refreshes of the same cache path

Recommended v1 approach:

- use best-effort file locking around metadata writes
- use best-effort file locking per cache directory during create or refresh

If locking is not implemented in the first cut, metadata atomic rename still prevents torn writes, but last-writer-wins races remain possible. Adding lock files early is preferable.

## Security and Safety Considerations

- never execute arbitrary hooks or repo commands beyond normal Git operations required for clone, fetch, checkout, and status
- ensure destination paths remain within the tasktree root
- reject dangerous names like `..` or path separators in `--name` unless intentionally supported
- never delete cache data during `remove`
- avoid mutating unrelated directories outside the resolved tasktree

## Testing Strategy

Testing should combine unit tests and black-box integration tests.

### Unit tests

Focus on:

- upward root resolution
- repo name derivation from URL
- cache path hashing determinism
- metadata serialization and round-trip parsing
- atomic write helpers
- path safety validation

### Integration tests

Use temporary directories and locally created Git repos to test:

- `init` creates valid metadata
- `add` from default branch
- `add --ref <branch>`
- `add --ref <tag>` with detached HEAD
- `add --branch <name>` from default branch
- `add --ref X --branch Y`
- duplicate names fail cleanly
- remove deletes checkout and updates metadata only
- status reflects dirty and clean states
- root resolution works from nested repo subdirectories

### Test implementation notes

- prefer creating throwaway local bare remotes in temp directories
- avoid relying on external networked Git hosts in automated tests
- keep Git wrapper commands injectable for easier stubbing in unit tests

## Observability and Debuggability

V1 does not need telemetry, but it should be easy to debug locally.

Recommended practices:

- keep metadata human-readable
- preserve Git stderr in surfaced errors
- add a verbose mode later if needed
- keep command wrappers centralized so logging can be added in one place

## Future-Friendly Design Choices

This design leaves room for future additions without changing the core model:

- `tasktree info [name]`
- `tasktree doctor`
- cache pruning commands
- machine-readable output modes
- optional global config for defaults
- cross-repo execution commands

The key invariant that should not change is that a tasktree remains a normal directory with a single metadata file and normal Git checkouts.

## Recommended v1 Implementation Plan

1. bootstrap Go module, CLI skeleton, and Devbox config
2. implement tasktree root resolution and metadata store
3. implement `init`
4. implement Git wrapper and cache manager
5. implement `add`
6. implement `list` and `root`
7. implement `remove`
8. implement `status`
9. add integration tests around temp repos and caches
10. refine error messages and output formatting

## Final Recommendation

Build Tasktree as a small Go CLI that shells out to native Git, stores minimal TOML metadata atomically, and uses deterministic hashed bare-repo caches under the user cache directory.

Use `devbox shell` to standardize the local contributor environment, but keep the runtime product as a normal standalone binary with no background services and no required configuration.
