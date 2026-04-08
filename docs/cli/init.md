# tasktree init

Initialize a new tasktree workspace.

## Synopsis

```
tasktree init [path]
```

## Description

Creates a `Tasktree.yml` file in the target directory (defaulting to the current directory) and registers the workspace in the global registry at `~/.local/state/tasktree/registry.toml`.

If `path` does not exist, it is created.

## Arguments

| Argument | Description |
|---|---|
| `path` | Directory to initialize. Defaults to `.` (current directory). |

## Examples

Initialize the current directory:

```bash
cd ~/work/feature-payments
tasktree init
# Initialized tasktree at /home/user/work/feature-payments
```

Initialize a new directory:

```bash
tasktree init ~/work/feature-payments
# Initialized tasktree at /home/user/work/feature-payments
```

## Errors

| Error | Cause |
|---|---|
| `tasktree already initialized at <path>` | A `Tasktree.yml` already exists at the target path. |
