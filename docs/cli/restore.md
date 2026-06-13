# tasktree restore

Reproduce a workspace from a snapshot `.tar.gz` onto a fresh directory.

## Synopsis

```
tasktree restore <snapshot> [--into <dir>] [--skip-bootstrap]
```

## Description

Reproduces the exact working state captured by [`tasktree snapshot`](snapshot.md): it re-materializes sources pinned to the recorded commits, replays local commits from the bundle, restores dirty edits, and then runs the workspace bootstrap steps.

The snapshot is self-contained (it embeds `Tasktree.yml`), so restore needs nothing but the tarball.

Pass `-` to read the snapshot from **stdin**.

## Arguments

| Argument | Description |
|---|---|
| `<snapshot>` | Path to a snapshot `.tar.gz`, or `-` to read from stdin. |

## Flags

| Flag | Description |
|---|---|
| `--into <dir>` | Target directory. Defaults to `./<name>` from the embedded spec. The target must be empty or not yet exist. |
| `--skip-bootstrap` | Restore the working tree only; do not run bootstrap steps. |

## How it works

Restore is split by source type:

- **Non-git sources** are re-materialized from the embedded spec.
- **Git sources** follow a deterministic, pinned path:
    1. clone via the local cache and point `origin` at the recorded remote URL,
    2. ensure the base commit is present (fetching from the remote if needed),
    3. replay local commits from the bundle,
    4. recreate the branch (or detached HEAD) at the recorded commit,
    5. **verify** that HEAD matches the recorded SHA, failing loudly on mismatch,
    6. unpack the dirty archive, apply deletions, and re-stage the recorded staged paths.

The working tree is reconstructed in a temporary staging directory and atomically moved into place on success, so a failed restore never leaves a partial workspace. After the move, the workspace is registered in the [global registry](../concepts/registry.md) and bootstrap runs (unless `--skip-bootstrap`).

!!! note "Why bootstrap runs on restore"
    Bootstrap outputs (`node_modules`, generated config) are normally `.gitignore`d and excluded from the snapshot, so a freshly restored workspace would otherwise have code but no installed dependencies. Running bootstrap (idempotent by contract) makes "restore → working workspace" the default outcome. Bootstrap is fail-fast with no rollback, exactly like `apply`.

## Examples

Restore into `./<name>`:

```bash
tasktree restore feature.tar.gz
# Restored "feature" to /home/laci/work/feature
```

Restore into a chosen directory:

```bash
tasktree restore feature.tar.gz --into ./reproduced
```

Pipe straight from another host:

```bash
tasktree snapshot -o - | ssh host 'tasktree restore - --into work'
```

Restore without running bootstrap:

```bash
tasktree restore feature.tar.gz --skip-bootstrap
```

## Notes

- Restore refuses a non-empty target directory. Choose an empty or new path.
- A snapshot produced by a newer, incompatible `tasktree` (unknown manifest version) is rejected with a clear error — upgrade tasktree.
- Restore relies on the recorded remote being reachable to reconstruct the base commit. If the remote is unavailable or the base commit was force-removed, restore fails.
