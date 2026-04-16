# CLI Reference

## Global flag

| Flag | Short | Description |
|---|---|---|
| `--verbose` | `-v` | Print each git command invoked to stderr |

## Context resolution

Most commands that operate on a workspace (`add`, `remove`, `repos`, `status`, `apply`) resolve the tasktree root by walking up from the current working directory until a `Tasktree.yml` file is found. You do not need to `cd` to the root before running these commands.

If no `Tasktree.yml` is found, the command exits with:

```
Error: not in a tasktree (no Tasktree.yml found walking up from <path>)
```

If a legacy `.tasktree.toml` is found instead:

```
Error: found legacy .tasktree.toml at <path> — run 'tasktree migrate' to convert
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Any error (domain error, git failure, filesystem error) |

## Commands

| Command | Description |
|---|---|
| [`init`](init.md) | Initialize a new tasktree workspace |
| [`add`](add.md) | Add a repository to the current tasktree |
| [`apply`](apply.md) | Materialize sources declared in Tasktree.yml |
| [`remove`](remove.md) | Remove a repository from the current tasktree |
| [`repos`](repos.md) | List repositories declared in the current tasktree |
| [`status`](status.md) | Show live git status for all repositories |
| [`annotate`](annotate.md) | Manage annotations for the current tasktree |
| [`list`](list.md) | List all known tasktrees on this machine |
| [`root`](root.md) | Print the current tasktree root path |
| [`prune`](prune.md) | Remove stale entries from the global registry |
| [`migrate`](migrate.md) | Convert a legacy `.tasktree.toml` to `Tasktree.yml` |
| [`repo`](repo.md) | Manage repository aliases |
