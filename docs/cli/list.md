# tasktree list

List all known tasktrees on this machine.

## Synopsis

```
tasktree list
```

## Description

Reads the global registry at `~/.local/state/tasktree/registry.toml` and prints all registered tasktrees. This is a machine-wide view, not scoped to the current directory.

Entries whose path no longer exists or no longer contains a `Tasktree.yml` are shown with a `missing` status. Use [`tasktree prune`](prune.md) to remove them.

## Output

```
NAME                PATH                              STATUS
feature-payments    /home/user/feature-payments
hotfix-login        /home/user/hotfix-login
old-workspace       /home/user/deleted-workspace      missing
```

## Examples

```bash
tasktree list
```
