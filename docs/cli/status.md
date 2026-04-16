# tasktree status

Show live git status for all repositories in the current tasktree.

## Synopsis

```
tasktree status
```

## Description

For each source declared in `Tasktree.yml`, queries the actual git working copy and reports the current HEAD and working tree state. Unlike [`tasktree repos`](repos.md), this reflects the live on-disk state, not the declared intent.

If the workspace has any annotations set, they are displayed in the header above the repository table.

## Output

Without annotations:

```
Tasktree: feature-payments
Root:     /home/user/feature-payments

REPO              PATH              HEAD                STATE
api               api               feature/payments    clean
web               web               feature/payments    dirty
payments-service  payments-service  v1.4.0 (HEAD)       clean
```

With annotations:

```
Tasktree: feature-payments
Root:     /home/user/feature-payments

  owner    team-payments
  purpose  Integration testing for Q3 payments feature
  ticket   JIRA-4821

REPO              PATH              HEAD                STATE
api               api               feature/payments    clean
web               web               feature/payments    dirty
payments-service  payments-service  v1.4.0 (HEAD)       clean
```

| Column | Description |
|---|---|
| `REPO` | Source name |
| `PATH` | Checkout path relative to the tasktree root |
| `HEAD` | Current branch name, or HEAD description for detached HEAD state |
| `STATE` | `clean` if the working tree has no uncommitted changes; `modified` if there are staged or unstaged modifications |

## Examples

```bash
tasktree status
```

Run from any directory inside the tasktree — context resolution walks up to find the root.

