# Snapshots

A **snapshot** is a lightweight, portable, self-contained capture of a workspace's *concrete* state. It is the complement to `Tasktree.yml`.

## Desired state vs. concrete state

`Tasktree.yml` is **pure desired state**: which repositories, which branches, which refs. It deliberately stores no resolved commit SHAs and no machine-specific paths, so it stays safe to commit and `apply` stays idempotent. It cannot represent "where I actually am right now."

A snapshot fills that gap. It is the one artifact where resolved base SHAs and uncommitted edits are allowed to live:

| | `Tasktree.yml` | Snapshot |
|---|---|---|
| Captures | desired state (intent) | concrete state (this instant) |
| Resolved SHAs | never | yes (base + head pins) |
| Uncommitted edits | never | yes (dirty archive) |
| Reproduced with | `tasktree apply` | `tasktree restore` |
| Commit to git? | yes | no — it's a build/transport artifact |

## The artifact

A snapshot is a single `.tar.gz` with this internal layout:

```
snapshot.yaml            # manifest (resolved pins + which payloads exist)
Tasktree.yml             # embedded copy of the spec → self-contained restore
bundles/<source>.bundle  # git: local commits base..HEAD (only if any exist)
dirty/<source>.tar       # git: working-tree content of dirty paths (only if dirty)
```

Because the spec is embedded, a snapshot can be restored on a fresh machine with nothing else on hand.

## What a git source captures

| Part | Encoding | Notes |
|---|---|---|
| Base | resolved `merge-base` against the remote + remote URL, in the manifest | Re-materializable from the remote at that pin. Always recorded, even when clean. |
| Committed delta | a `git bundle` of `base..HEAD` | Local commits beyond the base. Handles binaries and merge history. |
| Dirty state | a tar of the working-tree content of changed + untracked files | Captures content (not diffs), preserving binaries and file modes. `.gitignore` respected by default; `--include-ignored` opts in. |

Non-git sources (`http`, `archive`, `static`, `local`) carry no payload — they are reproduced from the embedded spec on restore.

## Restore

[`tasktree restore`](../cli/restore.md) reproduces the working tree deterministically: re-materialize sources pinned to the recorded commits, replay local commits from the bundle, recreate the branch (or detached HEAD) at the recorded SHA, verify HEAD matches, then unpack dirty edits. The working tree is staged in a temp directory and atomically moved into place, the workspace is registered, and bootstrap runs (unless skipped).

## When to use which

- **Share a workspace setup** (which repos/branches) → commit and share `Tasktree.yml`, reproduce with [`apply`](../cli/apply.md). See [Sharing a Workspace](../patterns/applying-from-spec.md).
- **Reproduce an exact working tree** (precise commits + in-progress edits), e.g. handing off mid-task work or returning a result from a sandbox → use [`snapshot`](../cli/snapshot.md) / [`restore`](../cli/restore.md).

## Limitations (v0)

- Captures the current branch/HEAD only (not all local branches).
- Partial staging collapses to the working-tree content (index-vs-worktree divergence on the same path resolves to the worktree version); the staged path set is recorded as a re-stage hint.
- A git source with no `origin` remote cannot be snapshotted.
- Git submodules' own working trees are not captured.
- Restore relies on the recorded remote being reachable to reconstruct the base commit.
