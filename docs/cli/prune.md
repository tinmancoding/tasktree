# tasktree prune

Remove stale entries from the global registry.

## Synopsis

```
tasktree prune [flags]
```

## Description

Scans the global registry at `~/.local/state/tasktree/registry.toml` and removes entries that are no longer valid — either because the path no longer exists on disk, or because the path no longer contains a `Tasktree.yml` file.

## Flags

| Flag | Description |
|---|---|
| `--dry-run` | Preview which entries would be removed without modifying the registry. |

## Examples

Remove stale entries:

```bash
tasktree prune
```

```
Removed old-workspace (/home/user/deleted-workspace)
```

Preview without changes:

```bash
tasktree prune --dry-run
```

```
Would remove old-workspace (/home/user/deleted-workspace, missing)
```

When nothing needs pruning:

```bash
tasktree prune
# Nothing to prune.
```
