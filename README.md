# tasktree

Tasktree is a task-first workspace manager for local development across one or more Git repositories.

It creates a directory with a `.tasktree.toml` file plus one or more normal Git checkouts. Clones are accelerated through deterministic local bare-repo caches under the user cache directory.

## Current commands

- `tasktree init [path]`
- `tasktree add <repo-url> [--ref <ref>] [--branch <branch>] [--name <name>]`
- `tasktree list`
- `tasktree status`
- `tasktree remove <name>`
- `tasktree root`

## Quickstart

```bash
mkdir -p ~/ws/feature-payments
cd ~/ws/feature-payments
tasktree init
tasktree add /path/to/repo.git --branch feature/payments
tasktree list
tasktree status
```

`tasktree` commands that operate on the current workspace resolve the tasktree root by walking upward from the current directory, so they work from nested repo subdirectories too.

## Development

```bash
devbox shell
go test ./...
go run ./cmd/tasktree --help
```

## Releases

GitHub Actions uses Release Please and GoReleaser to manage version tags, GitHub releases, and downloadable binaries.

- The configuration starts tracking releases from the commit where this workflow was added.
- Merge changes using Conventional Commits such as `feat:`, `fix:`, or `chore:`.
- The release workflow opens or updates a release PR with the next version and changelog.
- When that release PR is merged, Release Please creates the tag and GitHub release.
- A separate GoReleaser workflow runs on `v*` tags and uploads cross-platform archives plus `checksums.txt` to that release.

## Notes

- Git operations use the system `git` binary.
- Metadata is stored in `.tasktree.toml` and written atomically.
- Removing a repo checkout does not remove its local bare cache.
