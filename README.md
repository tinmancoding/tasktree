# tasktree

Tasktree is a task-first workspace manager for local development across one or more Git repositories.

It creates a workspace directory with a `.tasktree.toml` file plus one or more normal Git checkouts. Repeated clones stay fast because tasktree keeps deterministic local bare-repo caches under the user cache directory.

## Features

- Create a dedicated workspace for a task, bug fix, or feature branch
- Add one or many Git repositories to the same tasktree
- Clone from a repo's default branch, a specific ref, or a new local branch
- Reuse cached bare clones for faster subsequent checkouts
- Run tasktree commands from the workspace root or any nested repo subdirectory
- Inspect all workspace repositories with `list` and `status`
- Remove a checkout without touching the shared bare cache
- Manage repository aliases in `~/.config/tasktree/repos.yml`
- Auto-register useful aliases when a repo is added

## Getting Started

### Install

If you are building from source:

```bash
go install github.com/tinmancoding/tasktree/cmd/tasktree@latest
```

Or in this repository:

```bash
go build ./cmd/tasktree
```

Make sure the `tasktree` binary is on your `PATH`.

### Basic Workflow

```bash
mkdir -p ~/ws/feature-payments
cd ~/ws/feature-payments

tasktree init
tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/web.git --branch feature/payments

tasktree list
tasktree status
```

## Tutorial

This walkthrough shows the typical flow for starting work on a feature that touches multiple repositories.

### 1. Create a workspace

```bash
mkdir -p ~/ws/feature-checkout
cd ~/ws/feature-checkout
tasktree init
```

This creates the tasktree root and writes `.tasktree.toml`.

### 2. Add repositories

Add a repo from its default branch:

```bash
tasktree add git@github.com:myorg/api.git
```

Example output:

```text
Added api at api
Registered alias api -> git@github.com:myorg/api.git
Registered alias myorg-api -> git@github.com:myorg/api.git
```

Add a repo and create a local branch from the resolved starting point:

```bash
tasktree add git@github.com:myorg/web.git --branch feature/checkout
```

Add a repo from a tag, branch, or commit:

```bash
tasktree add git@github.com:myorg/payments.git --ref v1.2.0
tasktree add git@github.com:myorg/payments.git --ref main --branch feature/checkout
```

Add a repo with a custom checkout directory name:

```bash
tasktree add git@github.com:myorg/api.git --name api-v2
```

### 3. See what was created

```bash
tasktree list
tasktree status
```

Example `tasktree list` output:

```text
NAME  PATH  REF    BRANCH
api   api   main   feature/checkout
web   web   main   feature/checkout
```

Example `tasktree status` output:

```text
Tasktree: feature-checkout
Root: /Users/alex/ws/feature-checkout

REPO  PATH  HEAD              STATE
api   api   feature/checkout  clean
web   web   feature/checkout  modified
```

- `list` shows configured repositories, checkout ref, and branch
- `status` shows the tasktree name, root path, current HEAD, and whether each repo is clean or modified

### 4. Use aliases to make future adds shorter

When you add a repo by URL, tasktree automatically tries to register two aliases in `~/.config/tasktree/repos.yml`:

- `repo-name`
- `owner-repo-name`

For `git@github.com:myorg/api.git`, tasktree will try to register:

- `api`
- `myorg-api`

If an alias is already used by another repository, tasktree skips it and prints a message explaining why.

Example conflict output:

```text
Added app at app
Skipped alias app; already used by git@github.com:someone-else/app.git
Registered alias myorg-app -> git@github.com:myorg/app.git
```

After aliases exist, you can add by alias instead of full URL:

```bash
tasktree add api
tasktree add myorg-web --branch feature/checkout
```

You can also manage aliases explicitly:

```bash
tasktree repo add-alias payments git@github.com:myorg/payments.git
tasktree repo aliases
tasktree repo remove-alias payments
```

### 5. Remove a checkout you no longer need

```bash
tasktree remove web
```

This removes the checkout from the current tasktree and updates metadata. It does not remove the shared bare cache.

### 6. Run commands from anywhere inside the workspace

Tasktree commands resolve the workspace root by walking upward from the current directory, so this works from nested repo paths too:

```bash
cd ~/ws/feature-checkout/api/internal/server
tasktree root
tasktree status
```

## CLI Reference

### Global Flags

- `-v, --verbose`: print underlying git commands to stderr

### `tasktree init [path]`

Initialize a tasktree in the current directory or the provided path.

Examples:

```bash
tasktree init
tasktree init ~/ws/feature-checkout
```

### `tasktree add <repo-url> [--ref <ref>] [--branch <branch>] [--name <name>]`

Add a repository to the current tasktree.

Accepted input:

- a clone URL such as `git@github.com:myorg/api.git`
- a configured alias such as `api`

Flags:

- `--ref <ref>`: start from a branch, tag, commit, or ref
- `--branch <branch>`: create a new local branch from the resolved starting point
- `--name <name>`: use a custom checkout directory name

Examples:

```bash
tasktree add git@github.com:myorg/api.git
tasktree add git@github.com:myorg/api.git --ref main
tasktree add git@github.com:myorg/api.git --ref v1.2.0
tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/api.git --ref main --branch feature/payments
tasktree add git@github.com:myorg/api.git --name api2
tasktree add api
```

What `add` does:

- resolves the current tasktree root
- resolves the input as an alias if one exists
- uses the shared local bare-repo cache when cloning
- creates a normal checkout inside the tasktree
- records the repo in `.tasktree.toml`
- attempts to register derived aliases in `repos.yml`
- prints what aliases were added, already existed, or were skipped due to conflicts

### `tasktree list`

List repositories configured in the current tasktree.

Output columns:

- `NAME`
- `PATH`
- `REF`
- `BRANCH`

### `tasktree status`

Show live status for repositories in the current tasktree.

Output includes:

- tasktree name
- tasktree root
- one row per repo with `REPO`, `PATH`, `HEAD`, and `STATE`

### `tasktree remove <name>`

Remove a repository checkout from the current tasktree.

This removes the working checkout and updates `.tasktree.toml`. It does not remove shared cached clones.

### `tasktree root`

Print the resolved root path for the current tasktree.

### `tasktree repo add-alias <alias> <clone-url>`

Add a manual alias for a repository URL.

Rules:

- adding the same alias for the same repo is a no-op
- adding an alias already used by a different repo fails

Example:

```bash
tasktree repo add-alias api git@github.com:myorg/api.git
```

### `tasktree repo remove-alias <alias>`

Remove an alias from the global alias catalog.

Example:

```bash
tasktree repo remove-alias api
```

### `tasktree repo aliases`

List all configured aliases and their clone URLs.

Example:

```bash
tasktree repo aliases
```

### `tasktree completion <shell>`

Generate shell completion scripts.

Example:

```bash
tasktree completion bash
tasktree completion zsh
```

## Configuration

### Tasktree Metadata

Each workspace stores local metadata in:

```text
.tasktree.toml
```

This file tracks the tasktree name, creation time, and repositories in the workspace.

### Global Repository Alias Catalog

Tasktree stores repository aliases in the default config directory, usually:

```text
~/.config/tasktree/repos.yml
```

Structure:

```yaml
repos:
  - url: git@github.com:myorg/api.git
    aliases:
      - api
      - myorg-api
  - url: git@github.com:myorg/web.git
    aliases:
      - web
      - myorg-web
```

Notes:

- aliases are global across tasktrees
- an alias can only point to one repository URL
- a repo entry may exist even if it currently has no aliases

## How Caching Works

Tasktree keeps a deterministic local bare clone cache under the user cache directory, typically something like:

```text
~/.cache/tasktree/repos
```

When you add the same repository again in another tasktree, tasktree refreshes the bare cache and clones from it instead of cloning from the network every time.

## Development

```bash
go test ./...
go run ./cmd/tasktree --help
```

## Releases

GitHub Actions uses Release Please and GoReleaser to manage version tags, GitHub releases, and downloadable binaries.

- Merge changes using Conventional Commits such as `feat:`, `fix:`, or `chore:`
- Release Please opens or updates a release PR with the next version and changelog
- Merging that PR creates the tag and GitHub release
- GoReleaser publishes cross-platform archives and `checksums.txt`

## Notes

- Git operations use the system `git` binary
- Metadata is written atomically
- Removing a repo checkout does not remove its local bare cache
