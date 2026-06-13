# tasktree snapshot

Capture the workspace's concrete state into a single, portable `.tar.gz`.

## Synopsis

```
tasktree snapshot [-o <path>] [--include-ignored]
```

## Description

While `Tasktree.yml` is pure **desired** state (no resolved SHAs, safe to commit), a snapshot is its complement: the **concrete** working state as it is *right now* — the exact base commits, any local commits, and uncommitted edits — captured so the working tree can be reproduced on another machine or at a later time with [`tasktree restore`](restore.md).

The result is a single, self-contained `.tar.gz`. See [Snapshots](../concepts/snapshots.md) for the artifact layout and data model.

For each `git` source the snapshot records:

- **Base pin** — the resolved base commit (`merge-base` against the remote) plus the remote URL, in the manifest. Always pinned, even for a clean source.
- **Committed delta** — a `git bundle` of local commits beyond the base (only when present).
- **Dirty state** — a tar of the working-tree content of changed + untracked files (only when dirty). `.gitignore` is respected by default.

Non-git sources (`http`, `archive`, `static`, `local`) carry no payload — they are reproduced from the embedded spec on restore.

## Flags

| Flag | Description |
|---|---|
| `-o, --output <path>` | Where to write the snapshot. Defaults to `./<name>-<UTC-timestamp>.tar.gz`. Use `-o -` to stream the archive to **stdout**. |
| `--include-ignored` | Also capture `.gitignore`d files in the dirty archive (e.g. build output). Off by default to keep snapshots small. |

## Preconditions

- The command resolves the workspace root by walking up from the current directory, like `apply`.
- It **fails** if a declared source is not yet materialized — run [`tasktree apply`](apply.md) first.
- A fully clean workspace produces a valid snapshot (base pins only, no bundle or dirty payloads).

## Examples

Capture the current workspace to a timestamped file:

```bash
tasktree snapshot
# Wrote snapshot to demo-20260605T120000Z.tar.gz (2 sources)
```

Write to a specific file:

```bash
tasktree snapshot -o /tmp/feature.tar.gz
```

Stream to stdout to pipe straight into `restore` on another host:

```bash
tasktree snapshot -o - | ssh host 'tasktree restore - --into work'
```

Include ignored files:

```bash
tasktree snapshot --include-ignored
```

## Notes

- The output is written atomically (to a temp file, then renamed), so an interrupted snapshot never leaves a half-written `.tar.gz`. Streaming to stdout (`-o -`) skips this.
- A git source with no `origin` remote cannot be snapshotted (its base would be unrecoverable on restore); the command fails with a clear error.
- Git submodules are out of scope: the submodule pointer is captured as part of the tree, but the submodule's own working tree is not.
