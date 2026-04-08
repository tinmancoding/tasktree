# tasktree

Tasktree is a task-first workspace manager for local development across one or more Git repositories.

It creates a workspace directory with a `Tasktree.yml` file plus one or more normal Git checkouts. Repeated clones stay fast because tasktree keeps deterministic local bare-repo caches under the user cache directory.

## Features

- Create a dedicated workspace for a task, bug fix, or feature branch
- Add one or many Git repositories to the same tasktree
- Clone from a repo's default branch, an existing branch, or a new local branch
- Reuse cached bare clones for faster subsequent checkouts
- Run tasktree commands from the workspace root or any nested repo subdirectory
- Inspect all workspace repositories with `repos` and `status`
- List all known tasktrees on this machine with `list`
- Remove stale registry entries with `prune`
- Remove a checkout without touching the shared bare cache
- Manage repository aliases in `~/.config/tasktree/repos.yml`
- Auto-register useful aliases when a repo is added
- Migrate from the legacy `.tasktree.toml` format with `tasktree migrate`
- Reproduce a workspace on any machine with `tasktree apply`

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

tasktree repos
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

This creates the tasktree root and writes `Tasktree.yml`.

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

Add a repo and check out or create a branch:

```bash
tasktree add git@github.com:myorg/web.git --branch feature/checkout
```

`--branch` reuses a local branch if it exists, tracks the remote branch if only `origin/<branch>` exists, or creates a new branch from `--from` (or the default branch) if neither exists.

Add a repo from a tag or commit (headless checkout):

```bash
tasktree add git@github.com:myorg/payments.git --from v1.2.0
tasktree add git@github.com:myorg/payments.git --from 8f3e2ab
```

Create a new branch from an explicit base ref:

```bash
tasktree add git@github.com:myorg/payments.git --branch feature/checkout --from main
```

Add a repo with a custom checkout directory name:

```bash
tasktree add git@github.com:myorg/api.git --name api-v2
```

### 3. See what was created

```bash
tasktree repos
tasktree status
```

Example `tasktree repos` output:

```text
NAME  PATH  REF               BRANCH
api   api   feature/checkout  feature/checkout
web   web   feature/checkout  feature/checkout
```

Example `tasktree status` output:

```text
Tasktree: feature-checkout
Root: /Users/alex/ws/feature-checkout

REPO  PATH  HEAD              STATE
api   api   feature/checkout  clean
web   web   feature/checkout  modified
```

- `repos` shows configured repositories in the current tasktree, including checkout ref and branch
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
tasktree add myorg-web --branch feature/checkout```

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

### 7. Share or reproduce a workspace

`Tasktree.yml` is pure desired state — no resolved commit SHAs, no machine-specific paths. It is safe to commit to version control or share with teammates.

To reproduce a workspace on a new machine, copy `Tasktree.yml` to a directory and run `tasktree apply`:

```bash
mkdir ~/ws/feature-checkout-copy
cp ~/ws/feature-checkout/Tasktree.yml ~/ws/feature-checkout-copy/
cd ~/ws/feature-checkout-copy
tasktree apply
```

`apply` clones every source in the spec that is not yet present on disk. Sources already present are skipped, so `apply` is safe to run repeatedly.

## CLI Reference

### Global Flags

- `-v, --verbose`: print underlying git commands to stderr

### `tasktree init [path]`

Initialize a tasktree in the current directory or the provided path. Writes `Tasktree.yml`.

Examples:

```bash
tasktree init
tasktree init ~/ws/feature-checkout
```

### `tasktree add <repo-url> [--branch <branch>] [--from <ref>] [--name <name>]`

Add a repository to the current tasktree.

Accepted input:

- a clone URL such as `git@github.com:myorg/api.git`
- a configured alias such as `api`

Flags:

- `--branch <branch>`: the branch to use. Reuses the branch if it already exists locally, creates a local tracking branch if only `origin/<branch>` exists, or creates a new local branch from `--from` (falling back to the repo default branch) if neither exists.
- `--from <ref>`: base ref for branch creation when `--branch` is provided but the branch does not yet exist. When `--branch` is omitted, `--from` performs a direct checkout of the given branch, tag, commit, or ref without creating a new branch.
- `--name <name>`: use a custom checkout directory name

Examples:

```bash
# Check out default branch
tasktree add git@github.com:myorg/api.git

# Use an existing local or remote branch
tasktree add git@github.com:myorg/api.git --branch feature/payments

# Create a new branch from an explicit base
tasktree add git@github.com:myorg/api.git --branch feature/payments --from main

# Headless checkout of a tag, commit, or ref
tasktree add git@github.com:myorg/api.git --from v1.2.0
tasktree add git@github.com:myorg/api.git --from 8f3e2ab

# Custom checkout directory name
tasktree add git@github.com:myorg/api.git --name api2

# Use an alias
tasktree add api
```

What `--branch` does:

1. If the branch already exists locally — check it out; ignore `--from`.
2. Else if `origin/<branch>` exists — create a local tracking branch; ignore `--from`.
3. Else — resolve `--from` (or the repo default branch) and create the branch from there.

The command prints which path it took so ignored `--from` values are visible:

```text
Using existing local branch "feature/x".
Using existing remote branch "feature/x" from origin; ignoring --from "main".
Creating new branch "feature/x" from "main".
Checking out "v1.2.0" without creating a branch.
```

What `add` does:

- resolves the current tasktree root
- resolves the input as an alias if one exists
- uses the shared local bare-repo cache when cloning
- creates a normal checkout inside the tasktree
- records the source in `Tasktree.yml` under `spec.sources`
- attempts to register derived aliases in `repos.yml`
- prints what aliases were added, already existed, or were skipped due to conflicts

### `tasktree apply [--dry-run]`

Materialize all sources declared in `Tasktree.yml` that are not yet present on disk.

For each source in `spec.sources`:

- If the destination path already exists — skip it without error
- If the source type is `git` — populate the bare-clone cache and clone the repo, then apply the declared branch or ref
- If the source type is not yet supported — skip with a warning

Flags:

- `--dry-run`: preview what would be done without creating any directories

Examples:

```bash
# Reproduce a workspace from Tasktree.yml
tasktree apply

# See what would be cloned without doing anything
tasktree apply --dry-run
```

`apply` is idempotent: running it when all sources are already present prints "All sources are already present." and exits cleanly.

### `tasktree repos`

List repositories configured in the current tasktree.

Output columns:

- `NAME`
- `PATH`
- `REF`
- `BRANCH`

### `tasktree list`

List all tasktrees known to this machine.

Output columns:

- `NAME`
- `PATH`
- `STATUS` (omitted when OK; shows `missing` or `invalid` for stale entries)

### `tasktree prune [--dry-run]`

Remove stale entries from the global tasktree registry.

An entry is considered stale when:

- its path no longer exists on disk (`missing`)
- its path exists but no longer contains a `Tasktree.yml` (`invalid`)

Flags:

- `--dry-run`: preview what would be removed without modifying the registry

Examples:

```bash
tasktree prune
tasktree prune --dry-run
```

### `tasktree status`

Show live status for repositories in the current tasktree.

Output includes:

- tasktree name
- tasktree root
- one row per repo with `REPO`, `PATH`, `HEAD`, and `STATE`

### `tasktree remove <name>`

Remove a repository checkout from the current tasktree.

This removes the working checkout and updates `Tasktree.yml`. It does not remove shared cached clones.

### `tasktree migrate [path]`

Convert a legacy `.tasktree.toml` to `Tasktree.yml`.

Run this once in any workspace created with an older version of tasktree:

```bash
tasktree migrate
```

What it does:

1. Reads `.tasktree.toml` in the current directory (or `path` if provided)
2. Maps each `[[repos]]` entry to a `spec.sources` entry of type `git`
3. Writes `Tasktree.yml`
4. Renames `.tasktree.toml` to `.tasktree.toml.bak`

Resolved state fields (`resolved_ref`, `commit`) are intentionally discarded — live state is always queried from the Git checkouts directly.

Example output:

```text
Found .tasktree.toml in current directory.

Converting to Tasktree.yml...

  api          git  git@github.com:myorg/api.git  (branch: feature/checkout)
  web          git  git@github.com:myorg/web.git  (branch: feature/checkout)

Note: resolved_ref and commit fields are not carried over.
      Live state is always queried from the Git checkouts directly.

Written: Tasktree.yml
Renamed: .tasktree.toml → .tasktree.toml.bak

Migration complete. Review Tasktree.yml and commit it to version control.
```

If tasktree detects a `.tasktree.toml` without a `Tasktree.yml`, all commands (except `migrate`) will prompt you to run `tasktree migrate` first.

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

### Tasktree Metadata (`Tasktree.yml`)

Each workspace stores its desired state in a `Tasktree.yml` file at the workspace root. It uses a Kubernetes-style structure:

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree

metadata:
  name: feature-checkout
  createdAt: "2026-03-25T12:00:00Z"

spec:
  sources:
    - name: api
      type: git
      path: api
      git:
        url: "git@github.com:myorg/api.git"
        branch: feature/checkout

    - name: web
      type: git
      git:
        url: "git@github.com:myorg/web.git"
        branch: feature/checkout
```

`Tasktree.yml` is pure desired state — it contains no resolved commit SHAs, no timestamps written by the tool beyond `createdAt`, and no machine-specific paths. It is safe to commit to version control and share with teammates.

The tool appends or removes entries in `spec.sources` on `tasktree add` / `tasktree remove`. It never writes resolved state back into the file.

A JSON Schema for editor validation and autocompletion is available at `schema/tasktree.schema.json` in the tasktree repository.

### Migrating from `.tasktree.toml`

Workspaces created with older versions of tasktree use `.tasktree.toml`. Run `tasktree migrate` once to convert:

```bash
cd ~/ws/my-old-workspace
tasktree migrate
```

See [`tasktree migrate`](#tasktree-migrate-path) for full details.

### Global Tasktree Registry

Tasktree keeps a global registry of all initialized tasktrees at:

```text
~/.local/state/tasktree/registry.toml
```

This registry is updated automatically on `tasktree init` and is used by `tasktree list` to show all known workspaces. Stale entries (paths that no longer exist or have lost their `Tasktree.yml`) are reported in the `STATUS` column and can be cleaned up with `tasktree prune`.

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
