# tasktree init

Initialize a new tasktree workspace.

## Synopsis

```
tasktree init [path] [flags]
```

## Description

Creates a `Tasktree.yml` file in the target directory (defaulting to the current directory) and registers the workspace in the global registry at `~/.local/state/tasktree/registry.toml`.

If `path` does not exist, it is created.

## Arguments

| Argument | Description |
|---|---|
| `path` | Directory to initialize. Defaults to `.` (current directory). |

## Flags

| Flag | Description |
|---|---|
| `--annotate key=value` | Set an annotation at init time. Repeatable. Key must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`. |

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

Initialize with annotations:

```bash
tasktree init ~/work/feature-payments \
  --annotate purpose="Integration testing for Q3 payments" \
  --annotate owner=team-payments \
  --annotate ticket=JIRA-4821
```

## Errors

| Error | Cause |
|---|---|
| `tasktree already initialized at <path>` | A `Tasktree.yml` already exists at the target path. |
| `invalid annotation key "...": ...` | An `--annotate` key failed the format validation. |

