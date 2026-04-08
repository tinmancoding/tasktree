# tasktree remove

Remove a repository from the current tasktree.

## Synopsis

```
tasktree remove <name>
```

## Description

Deletes the checkout directory for the named source and removes its entry from `Tasktree.yml`.

The `<name>` argument must match the `name` field of a source in `Tasktree.yml`.

**This operation is destructive.** The checkout directory and all its contents are permanently deleted from disk. Any uncommitted changes are lost. Ensure you have committed or stashed all work before removing.

## Arguments

| Argument | Description |
|---|---|
| `name` | The source name as declared in `Tasktree.yml`. |

## Examples

```bash
tasktree remove api
# Removed /home/user/feature-payments/api
```

## Errors

| Error | Cause |
|---|---|
| `repo not found: <name>` | No source with this name exists in `Tasktree.yml`. |
| `unsafe path: <path>` | The resolved checkout path is outside the tasktree root (safety guard). |
