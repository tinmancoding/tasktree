# tasktree repos

List repositories declared in the current tasktree.

## Synopsis

```
tasktree repos
```

## Description

Reads `Tasktree.yml` and prints a table of all declared sources. This shows the **desired state** as written in the spec — not live git status. Use [`tasktree status`](status.md) for live information.

## Output

```
NAME              PATH              REF                 BRANCH
api               api               feature/payments    feature/payments
web               web                                   feature/payments
payments-service  payments-service  v1.4.0
```

| Column | Description |
|---|---|
| `NAME` | Source name from `Tasktree.yml` |
| `PATH` | Checkout path relative to the tasktree root |
| `REF` | `git.ref` field (base ref or pinned SHA/tag) |
| `BRANCH` | `git.branch` field |

## Examples

```bash
cd ~/work/feature-payments/api   # works from any subdirectory
tasktree repos
```
