# tasktree migrate

Convert a legacy `.tasktree.toml` to `Tasktree.yml`.

## Synopsis

```
tasktree migrate [path]
```

## Description

Reads `.tasktree.toml` in the given directory (defaulting to the current directory), converts all repo entries to the new `SourceSpec` YAML format, writes `Tasktree.yml`, and renames the old file to `.tasktree.toml.bak`.

Resolved-state fields (`resolved_ref`, `commit`) are intentionally discarded. Live state is always queried from Git working copies directly.

See the [Migration Guide](../getting-started/migration.md) for a full walkthrough.

## Arguments

| Argument | Description |
|---|---|
| `path` | Directory containing `.tasktree.toml`. Defaults to `.` (current directory). |

## Examples

```bash
cd ~/work/old-workspace
tasktree migrate
```

```
Found .tasktree.toml in /home/user/old-workspace.

Converting to Tasktree.yml...

  api                  git  git@github.com:myorg/api.git  (branch: feature/payments)
  web                  git  git@github.com:myorg/web.git

Note: resolved_ref and commit fields are not carried over.
      Live state is always queried from the Git checkouts directly.

Written: /home/user/old-workspace/Tasktree.yml
Renamed: .tasktree.toml → .tasktree.toml.bak

Migration complete. Review Tasktree.yml and commit it to version control.
```
