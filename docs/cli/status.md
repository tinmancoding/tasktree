# tasktree status

Show live git status for all repositories in the current tasktree.

## Synopsis

```
tasktree status
```

## Description

For each source declared in `Tasktree.yml`, queries the actual git working copy and reports the current HEAD and working tree state. Unlike [`tasktree repos`](repos.md), this reflects the live on-disk state, not the declared intent.

## Output

```
Tasktree: feature-payments  /home/user/feature-payments

NAME              PATH              HEAD                STATE
api               api               feature/payments    clean
web               web               feature/payments    dirty
payments-service  payments-service  v1.4.0 (HEAD)       clean
```

| Column | Description |
|---|---|
| `NAME` | Source name |
| `PATH` | Checkout path relative to the tasktree root |
| `HEAD` | Current branch name, or HEAD description for detached HEAD state |
| `STATE` | `clean` if the working tree has no uncommitted changes; `dirty` if there are staged or unstaged modifications |

## Examples

```bash
tasktree status
```

Run from any directory inside the tasktree — context resolution walks up to find the root.
