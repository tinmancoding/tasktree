# Tasktree PRD

## Overview

Tasktree is a task-first workspace manager for local development across one or more Git repositories.

A **tasktree** is simply:

- a directory
- containing a `.tasktree.toml` metadata file
- containing zero or more repository checkouts as subdirectories

Tasktree is **not** a Git worktree manager. It uses normal Git checkouts inside the tasktree directory. To speed up clone and checkout operations, Tasktree maintains **local cached bare repositories** under the user's cache directory and clones from those caches.

This design allows the same branch from the same repository to exist in multiple tasktrees in parallel, which is a key reason for not using Git worktrees in this tool.

---

## Problem Statement

Developers often need an isolated local workspace for a specific task, bugfix, or feature that may involve one or more repositories.

Current pain points:

- Local clones become disorganized over time.
- Multi-repo work often requires repetitive manual cloning and branch setup.
- Git worktrees do not fit all workflows because the same branch may need to exist in multiple independent task folders.
- Existing tools tend to be repo-centric or manifest-heavy instead of task-centric.

Tasktree should solve this by making it trivial to create a task-focused folder and add repositories into it with the desired ref or a new branch derived from that ref.

---

## Product Goals

### Primary goals

1. **Task-first workspace creation**
   - Users create a tasktree as a folder representing a unit of work.
   - Repositories are then added into that tasktree as needed.

2. **Zero-config usability**
   - Tasktree must work without any prior setup or global config.
   - A user should be able to initialize a tasktree and add repositories immediately.

3. **Fast repository materialization**
   - Repository checkouts should be accelerated by using cached local bare repositories.

4. **Support flexible checkout semantics**
   - Users can add a repository at a branch, tag, commit, or default branch.
   - Users can optionally create a new local branch from a given ref.

5. **Context awareness**
   - Commands should work when run from the tasktree root or any nested subdirectory inside it.

### Non-goals for v1

- Git worktree integration
- Shared team manifests
- Global repository catalogs
- Bundle/templates
- TUI/interactive interface
- Rich orchestration across multiple tasktrees
- Background daemon or persistent services

---

## Core Concepts

### Tasktree

A tasktree is a directory with a `.tasktree.toml` file.

Example:

```text
~/ws/feature-payments/
  .tasktree.toml
  api/
  web/
  contracts/
```

### Repository entry

A repository entry inside a tasktree is defined by:

- repository clone URL
- checkout path/name inside the tasktree
- requested checkout ref, if any
- resolved ref/commit
- optional created local branch

### Cache

Tasktree maintains cached local bare repositories under the user's cache directory.

Suggested default path:

```text
~/.cache/tasktree/repos/
```

Each remote repository gets one cached bare repository.

---

## User Stories

### Core user stories

1. As a developer, I want to initialize a tasktree in a folder so that I can organize work around a task.
2. As a developer, I want to add a repository to a tasktree by URL with no other setup.
3. As a developer, I want to optionally specify a branch, tag, or commit to check out.
4. As a developer, I want to create a new branch from a branch, tag, commit, or default branch at add time.
5. As a developer, I want commands to work even when I'm inside a nested subdirectory of a repo in the tasktree.
6. As a developer, I want to inspect the state of the tasktree and its repositories.
7. As a developer, I want repository cloning to be fast due to local caching.
8. As a developer, I want to remove a repository from a tasktree without affecting the cache.

---

## UX Principles

1. **Simple filesystem model**
   - A tasktree should be understandable just by looking at the folder.

2. **Explicit and boring CLI**
   - Prefer obvious command names over clever metaphors.

3. **No required setup**
   - The first-run experience should be frictionless.

4. **Safe by default**
   - Avoid destructive behavior unless explicitly requested.

5. **Debuggable state**
   - Metadata should make it easy to understand what happened.

---

## Command Line Interface

### Binary

Official binary name:

```text
tasktree
```

Optional user alias:

```bash
alias tt=tasktree
```

`tt` is not required as the official installed binary name.

---

## v1 Commands

### `tasktree init [path]`

Create a tasktree in the current directory or the specified path.

Examples:

```bash
tasktree init
tasktree init feature-payments
tasktree init ~/ws/feature-payments
```

Behavior:

- If no path is provided, initialize the current directory as a tasktree.
- If a path is provided and does not exist, create it.
- Create `.tasktree.toml` in the tasktree root.
- Fail if `.tasktree.toml` already exists unless an overwrite flag is added in the future.

### `tasktree add <repo-url>`

Add a repository checkout to the current tasktree.

Examples:

```bash
tasktree add git@github.com:myorg/api.git
tasktree add git@github.com:myorg/api.git --ref main
tasktree add git@github.com:myorg/api.git --ref v1.2.0
tasktree add git@github.com:myorg/api.git --ref 4f3c2ab
tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/api.git --ref main --branch feature/payments
tasktree add git@github.com:myorg/api.git --name api
```

Inputs:

- required: repo URL
- optional: `--ref <commitish>`
- optional: `--branch <new-branch-name>`
- optional: `--name <checkout-dir-name>`

Behavior:

- Must resolve the current tasktree by searching upward for `.tasktree.toml`.
- Must ensure a local bare cache exists for the repository.
- Must clone into the tasktree from the cache.
- Must derive the target directory name from the repo URL unless `--name` is provided.
- Must update `.tasktree.toml`.

### `tasktree remove <name>`

Remove a repository checkout from the current tasktree.

Examples:

```bash
tasktree remove api
tasktree remove web
```

Behavior:

- Resolve current tasktree from cwd upward.
- Remove the corresponding checkout directory from the tasktree.
- Remove the repo entry from `.tasktree.toml`.
- Do not remove or modify the cache.

### `tasktree list`

List repositories in the current tasktree.

Example:

```bash
tasktree list
```

Behavior:

- Resolve current tasktree from cwd upward.
- Show configured repositories and their basic metadata.

### `tasktree status`

Show status for repositories in the current tasktree.

Example:

```bash
tasktree status
```

Behavior:

- Resolve current tasktree from cwd upward.
- Show repo path, current HEAD/branch state, and dirty/clean state.

### `tasktree root`

Print the current tasktree root path.

Example:

```bash
tasktree root
```

Behavior:

- Search upward from the current directory for `.tasktree.toml`.
- Print the absolute tasktree root path.
- Fail with a clear error if not inside a tasktree.

---

## Future commands (not required for v1)

- `tasktree info [name]`
- `tasktree exec ...`
- `tasktree doctor`
- `tasktree clone`
- `tasktree rename`

---

## Context Awareness

Tasktree commands that operate on the current tasktree must be context-aware.

### Required behavior

When a command is run from:

- the tasktree root
- a repository directory inside the tasktree
- any nested subdirectory inside a repository inside the tasktree

Tasktree must search upward until it finds `.tasktree.toml`.

Example:

```bash
cd ~/ws/feature-payments/api/src/components
tasktree status
```

This must resolve to:

```text
~/ws/feature-payments/.tasktree.toml
```

### Error behavior

If no `.tasktree.toml` is found in the current directory or any parent directory, the command should fail with a clear message such as:

```text
Not inside a tasktree (no .tasktree.toml found in current directory or parents).
Run `tasktree init` to create one.
```

---

## Repository Add Semantics

### Inputs

`tasktree add` requires:

- repository clone URL

Optional inputs:

- `--ref <commitish>`
- `--branch <new-branch-name>`
- `--name <checkout-dir-name>`

### Definitions

#### Commitish

A commitish may be:

- branch name
- tag name
- commit SHA
- any ref Git can resolve

#### Default branch

If no `--ref` is provided, Tasktree must use the remote repository's default branch.

---

## `add` resolution rules

### Case 1: no `--ref`, no `--branch`

Example:

```bash
tasktree add git@github.com:myorg/api.git
```

Behavior:

- Resolve the remote default branch.
- Clone checkout into the tasktree.
- Check out the default branch.
- Record the resolved commit in metadata.

### Case 2: `--ref X`, no `--branch`

Example:

```bash
tasktree add git@github.com:myorg/api.git --ref v1.2.0
```

Behavior:

- Resolve `X`.
- Clone checkout into the tasktree.
- Check out the resolved target.
- Detached HEAD is acceptable if `X` is a tag or commit.
- If `X` is a branch, a normal branch checkout/tracking checkout may be used.

### Case 3: `--ref X --branch Y`

Example:

```bash
tasktree add git@github.com:myorg/api.git --ref main --branch feature/payments
```

Behavior:

- Resolve `X`.
- Clone checkout into the tasktree.
- Check out `X`.
- Create local branch `Y` from `X`.
- Switch checkout to local branch `Y`.

### Case 4: no `--ref`, `--branch Y`

Example:

```bash
tasktree add git@github.com:myorg/api.git --branch feature/payments
```

Behavior:

- Resolve remote default branch.
- Clone checkout into the tasktree.
- Check out default branch tip.
- Create local branch `Y` from default branch.
- Switch checkout to local branch `Y`.

---

## Naming and Paths

### Default checkout directory name

By default, Tasktree should derive the checkout directory name from the repository URL.

Examples:

- `git@github.com:myorg/api.git` → `api`
- `https://github.com/myorg/web.git` → `web`

### Override

Users may override the checkout directory name with:

```bash
--name <name>
```

Example:

```bash
tasktree add git@github.com:myorg/api.git --name api2
```

This creates:

```text
./api2
```

### Duplicate names

If the derived or specified name already exists in the tasktree metadata or as an existing non-empty directory, Tasktree should fail with a clear error and suggest using `--name`.

---

## Cache Design

### Cache location

Default cache root:

```text
~/.cache/tasktree/repos/
```

### Structure

Each repository URL maps to one bare cached repository.

Implementation may choose one of:

- human-readable sanitized names
- hashed names
- hybrid approach

The internal mapping format is an implementation detail, but it must be deterministic.

### Recommended cache behavior

When adding a repository:

1. Map repository URL to cache path.
2. If cache does not exist:
   - create it with `git clone --bare <url> <cache-path>`
3. If cache exists:
   - refresh it with `git fetch --all --prune`
4. Clone from cache into the tasktree checkout.
5. Check out the requested ref/branch.

### Notes

- Cache refresh happens on demand during `add`.
- No background service is required.
- Cache entries persist independently of tasktrees.

---

## Metadata File

Tasktree metadata is stored in:

```text
.tasktree.toml
```

### Goals for metadata

- human-readable
- easy to debug
- stores what the user requested and what was actually resolved
- avoids storing dynamic data that can be recomputed live

### Suggested schema (v1)

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

[[repos]]
name = "web"
path = "web"
url = "git@github.com:myorg/web.git"
checkout = "v1.4.0"
resolved_ref = "refs/tags/v1.4.0"
commit = "fedcba9876543210"
```

### Field definitions

#### Top-level

- `version`: metadata schema version
- `name`: human-friendly tasktree name
- `created_at`: RFC3339/ISO timestamp

#### Per-repo

- `name`: logical repo entry name
- `path`: relative path inside tasktree
- `url`: original clone URL
- `checkout`: user-requested ref, if any; may be default branch name when omitted
- `resolved_ref`: fully resolved Git ref if available
- `commit`: resolved commit SHA
- `branch`: created local branch name, if one was created

### Dynamic state

Tasktree should not persist highly dynamic state such as dirty/clean status in `.tasktree.toml` for v1. That information should be computed live by `status`.

---

## Output Expectations

### `tasktree list`

Suggested output:

```text
NAME        PATH        REF      BRANCH
api         api         main     feature/payments
web         web         v1.4.0   -
contracts   contracts   main     -
```

### `tasktree status`

Suggested output:

```text
Tasktree: feature-payments
Root: /home/me/ws/feature-payments

REPO        PATH        HEAD                STATE
api         api         feature/payments    clean
web         web         v1.4.0              detached, clean
contracts   contracts   main                modified
```

### `tasktree root`

Suggested output:

```text
/home/me/ws/feature-payments
```

---

## Error Handling and Edge Cases

### 1. Not inside a tasktree

Commands that require tasktree context must fail clearly when `.tasktree.toml` cannot be found.

### 2. Existing `.tasktree.toml` on init

`tasktree init` should fail rather than silently overwrite.

### 3. Duplicate repo name

If a repo entry name or destination path is already used in the tasktree, fail and suggest `--name`.

### 4. Existing destination directory

If the target checkout path already exists and is non-empty, fail clearly.

### 5. Same repository added multiple times

Allowed, provided the checkout name/path is different.

This is an explicit design goal because parallel checkouts are valid.

### 6. Detached HEAD

Allowed when adding a repository by tag or commit without `--branch`.

### 7. Nonexistent ref

If the provided `--ref` cannot be resolved, fail with a clear error including the repo URL and attempted ref.

### 8. Branch creation conflict

If `--branch` specifies a branch that already exists locally in the fresh checkout, fail clearly.

### 9. Cache refresh failure

If cache fetch/refresh fails, surface the Git error and abort the add operation.

### 10. Partial failure during add

If cloning or checkout fails after metadata is partially written, Tasktree must avoid leaving corrupt metadata behind.

Preferred behavior:
- perform repo setup first
- only write metadata after successful checkout
- or write atomically

---

## Implementation Requirements

### Required implementation characteristics

1. **Atomic metadata updates**
   - `.tasktree.toml` updates should be safe against interruption.

2. **Deterministic cache mapping**
   - the same URL must always map to the same cache path.

3. **Upward context resolution**
   - commands must work from subdirectories inside the tasktree.

4. **No global configuration dependency**
   - all v1 flows must work without config files outside the tasktree.

5. **Git as source of truth**
   - branch/ref/dirty state should be computed from Git, not inferred from stale metadata.

---

## Suggested Internal Algorithms

### Finding tasktree root

Pseudo-flow:

1. Start at current working directory.
2. Check for `.tasktree.toml`.
3. If found, return current directory as tasktree root.
4. Otherwise move to parent directory.
5. Repeat until filesystem root.
6. If not found, error.

### Add repository

Pseudo-flow:

1. Resolve tasktree root.
2. Parse `.tasktree.toml`.
3. Derive checkout name from URL unless overridden.
4. Ensure destination path does not already exist/conflict.
5. Resolve cache path from URL.
6. Create or refresh cache.
7. Clone from cache into destination directory.
8. Resolve target ref:
   - `--ref` if provided
   - remote default branch otherwise
9. Check out resolved ref.
10. If `--branch` provided, create and switch to local branch.
11. Determine resolved commit SHA and fully resolved ref if possible.
12. Update metadata atomically.

---

## Scope for v1

### Included

- `init`
- `add`
- `remove`
- `list`
- `status`
- `root`
- `.tasktree.toml`
- local bare cache
- default branch resolution
- optional ref checkout
- optional new branch creation
- upward context detection
- zero-config operation

### Excluded

- worktrees
- bundles
- global catalogs
- template repos
- per-user/global config
- advanced sync/orchestration
- TUI
- adoption of existing directories
- remote tasktree sharing

---

## Success Criteria

A successful v1 should let a new user do this with no setup:

```bash
mkdir ~/ws/feature-payments
cd ~/ws/feature-payments
tasktree init
tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/web.git --branch feature/payments
tasktree status
```

And get:

- a valid `.tasktree.toml`
- two working repository checkouts inside the tasktree
- local branch creation from the default branch
- fast subsequent clones due to caching
- correct behavior from any subdirectory inside the tasktree

---

## Future Evolution

Possible future enhancements after v1:

- `tasktree exec` to run commands across repos
- optional global config for defaults
- repo aliases/catalog
- tasktree templates
- richer inspection commands
- cleanup/prune cache commands
- better adoption/import of existing directories

These are explicitly secondary to shipping a clean zero-config v1.
