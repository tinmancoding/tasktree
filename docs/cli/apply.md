# tasktree apply

Materialize sources declared in `Tasktree.yml` that are not yet on disk.

## Synopsis

```
tasktree apply [flags]
```

## Description

Reads `Tasktree.yml` from the resolved tasktree root and ensures each declared source is present on disk. Sources whose destination path already exists are **skipped without error**, making `apply` safe to run repeatedly (idempotent).

The primary use case is reproducing a workspace from a committed `Tasktree.yml` — a teammate or CI runner runs `apply` after cloning the workspace repo.

Only `git` source types are currently implemented. Sources with other types are skipped with a notice.

## Flags

| Flag | Description |
|---|---|
| `--dry-run` | Preview what would be done without cloning or modifying anything. |

## Examples

Materialize all missing sources:

```bash
tasktree apply
```

```
Using existing remote branch "feature/payments" from origin.
Cloned api at api
Skipped web (already present)
```

Preview without making changes:

```bash
tasktree apply --dry-run
```

```
Would clone api at api (branch: feature/payments)
Skipped web (already present)
```

When all sources are already present:

```bash
tasktree apply
# All sources are already present.
```

## Branch resolution on apply

`apply` uses the same branch resolution logic as `add`:

1. `git.branch` exists locally → check it out
2. `git.branch` exists on `origin` → create a local tracking branch
3. `git.branch` does not exist → create from `git.ref` (or the remote default branch)
4. No `git.branch` set → check out `git.ref` or the default branch directly

## Notes

- `apply` does not update existing checkouts. It only materializes sources that are missing.
- To update an existing checkout, use `git fetch` / `git pull` inside the checkout directory.
