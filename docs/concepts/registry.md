# Global Registry

Tasktree maintains a global registry that tracks all tasktrees initialized on the machine. This enables `tasktree list` to show all workspaces regardless of current directory.

## Registry location

```
~/.local/state/tasktree/registry.toml
```

The file is managed automatically by `tasktree init` and `tasktree prune`. You should not need to edit it manually.

## How entries are added

Every time you run `tasktree init`, the new workspace is registered:

```bash
tasktree init ~/work/feature-payments
# → adds entry: path=~/work/feature-payments, name=feature-payments
```

## Listing all tasktrees

```bash
tasktree list
```

```
NAME                PATH                             STATUS
feature-payments    /home/user/feature-payments
hotfix-login        /home/user/hotfix-login
old-workspace       /home/user/deleted-workspace     missing
```

The `STATUS` column is blank for healthy entries. `missing` appears when the path no longer exists on disk or no longer contains a `Tasktree.yml`.

## Pruning stale entries

Over time, deleted or moved workspaces leave stale entries in the registry. Use `prune` to clean them up:

```bash
tasktree prune
```

```
Removed old-workspace (/home/user/deleted-workspace)
```

Use `--dry-run` to preview without making changes:

```bash
tasktree prune --dry-run
```

```
Would remove old-workspace (/home/user/deleted-workspace, missing)
```

## Registry file format

```toml
version = 1

[[tasktrees]]
path = "/home/user/feature-payments"
name = "feature-payments"
added_at = 2026-03-25T12:00:00Z

[[tasktrees]]
path = "/home/user/hotfix-login"
name = "hotfix-login"
added_at = 2026-03-27T09:15:00Z
```
