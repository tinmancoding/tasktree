# Sync

Without a remote, everything lives in `~/.tasktree`. With one, `tasktree sync` moves task records and tasktrees between machines through an ordinary git remote. This page explains how, and what happens when two machines disagree. For a walk-through, see [Two machines](../guides/two-machines.md).

## Setting up the remote

The remote is an empty git repository that you create anywhere you can push to: GitHub, a self-hosted server, or a bare repository on a disk. Only the tasks repository talks to it. Checkouts push to the local tasks repository, never to the remote.

```bash
tasktree config set remote git@github.com:myorg/tasks.git
```

This adds `origin` to the tasks repository and pushes `main` and every `task/*` branch in one atomic push. `tasktree config set remote ""` removes `origin` again. The remote only has to hold branches; it contains your task data, so make it private.

To join from a second machine, run `setup` with `--remote` on a machine that has no `~/.tasktree` yet. It clones the remote and creates the local branches for every task:

```bash
tasktree setup --remote git@github.com:myorg/tasks.git --author alice --machine desktop
```

If the second machine already has tasks of its own, `setup --remote` behaves like `config set remote` and pushes. The first `sync` then finds two `main` histories with no common commit and stops with `main histories are unrelated`. Rerun it with `--allow-unrelated`. Records have random ids, so files do not collide, and the semantic merge below applies. Handles can collide; `doctor` reports `duplicate-handle`.

## What `sync` does

Inside a tasktree, or with `-t`:

1. **Fetch** the remote. Without a remote, the sync is local and says `no hub remote configured`.
2. **Merge `main`.** Task records merge field by field (see below). The merged `main` is a merge commit named `sync: merge <machine>`.
3. **Fast-forward `task/<id>`**, in the hub and in the checkout. If both sides moved, sync stops with exit code 5 and tells you to run `resolve`.
4. **Apply pulled repository commits.** If the other machine moved a repository's recorded commit, a clean repository on its branch is fast-forwarded to it.
5. **Update repositories by mode**, with up to `sync.jobs` repositories fetched in parallel (default 4): `follow` and `work` repositories fast-forward to their `origin` branch. A failing repository is reported and the rest continue. Any gitlink moves this causes are committed on the task branch as `tasktree(<id>): sync update api abc123→def456`.
6. **Push** `main` and `task/<id>` to the remote with a lease on the state that was fetched. If another machine pushed in between, the lease fails and the whole sync runs again, up to three times.
7. **Report** what it skipped.

`sync` refuses to run, with exit code 5, when the root has uncommitted changes: `root has uncommitted changes; run tasktree commit first`. A repository left behind by a skipped gitlink makes the root look modified, but that state is recognized and does not count.

`sync` never pushes repository code to a repository's `origin`. Use [`tasktree push`](subtasks.md#returning-the-work).

### Without a tasktree

Outside a tasktree, `tasktree sync` is the same as `tasktree sync --all`: it merges `main`, fast-forwards every `task/*` branch between the tasks repository and the remote, and pushes. It never touches a checkout. It reports checkouts that are behind the hub and checkouts of closed tasks. A divergent task branch is listed and skipped, so one conflict does not block the others; the exit code is 5.

### `--no-repos`

`--no-repos` skips step 5, the updates from each repository's `origin`. Step 4 still runs: skipping it would let a later `tasktree commit` revert the other machine's repository commits.

### The report

```console
$ tasktree sync
fetched hub origin
reconciled main and task refs
pushed leased hub refs
Report:
  dirty: none
  diverged: none
  skipped:
    api: no origin/fix/PROJ-123
  unpushed gitlinks: none
  checkouts behind: none
  closed-task checkouts: none
  failed: none
```

| Section | Meaning |
|---|---|
| `dirty` | Repositories with uncommitted changes. They were not touched. |
| `diverged` | Task branches whose local and remote histories both moved |
| `skipped` | Repositories that did not move, with the reason: dirty, on another branch, `non-fast-forward`, or no matching branch on `origin` |
| `unpushed gitlinks` | A recorded commit that is not reachable from `origin`, or not yet available here. The repository stays on its tip, or empty, and the report says so. |
| `checkouts behind` | Checkouts whose root is behind the task branch in the hub |
| `closed-task checkouts` | Checkouts of tasks that are done or dropped, for example closed on another machine. They are never removed automatically. Run `tasktree mark <status> -t <handle>` with the task's closed status to remove one through the normal gates. |
| `failed` | Steps that failed. If any, the exit code is 1. |

`sync --json` prints the same report with keys `steps`, `dirty`, `diverged`, `skipped`, `unpushedGitlinks`, `checkoutsBehind`, `closedTaskCheckouts` and `failed`.

## Semantic merge of records

When `main` has diverged, tasktree merges every file `tasks/<id>.md` with a three-way merge against the common ancestor, not with a text merge:

| Part | Merge rule |
|---|---|
| Log | Union of entries by entry id. Entries from both sides are ordered by timestamp, then id. The bytes of your local entries are kept exactly. |
| `labels`, `refs` | Three-way set merge. An item added on either side is kept, an item removed on either side is dropped. |
| Scalar fields (`title`, `handle`, `status`, `parent`, unknown keys) | Three-way. A change on one side wins. A change on both sides to different values is a [conflict](#conflicts). |
| Description | Three-way on the whole text. A change on one side wins. A different change on both sides is a [conflict](#conflicts). |
| `updatedAt` | The later of the two |

Other files on `main` use git's normal three-way merge. A conflict there stops the sync before anything changes, with `main conflict on <path>`. If one side deleted a record and the other changed it, the changed version is kept.

The merge builds objects first and moves refs only when all files merged, so a failed sync leaves the hub as it was. If a sync is interrupted in the middle, `doctor` reports `hub-interrupted-sync`, and `doctor --fix` repairs it when there are no manual edits in the hub.

## Conflicts

A scalar or description conflict **keeps your local value**, so the record stays readable. tasktree then:

1. appends a `comment` log entry that holds the remote value and says where it came from: `Sync conflict on title: remote value (commit 5118de5 by Alice at 2026-10-13T16:33:27+02:00) was: Fix login redirect loop`,
2. adds the field to `conflict:` in the frontmatter.

The provenance is the last remote commit that touched the file. tasktree stores no machine metadata for it.

`list` shows `!` after the status, `info` prints `Conflict:` and `doctor` warns `conflict-flag`. Resolve it by deciding which value is right: `tasktree info set title "..."`, or `tasktree info ack title` to keep the local one. [Task records](task-records.md#conflict-flags) shows an example.

## Diverged task branches

Two machines can commit to the same tasktree. If one is fast-forwardable, sync takes it. If both moved, sync stops with exit code 5 and `task/<id> diverged`. The tool does not guess, because the branch holds files and exact repository commits.

```console
$ tasktree resolve
Local commits:
  1a71425 B adds extra
  7b82f0d B edits notes
Remote commits:
  905cf13 A edits notes
Changed on both sides (files and gitlinks):
  notes.md
Auto-merged paths:
  extra.md
error: paths changed on both sides; use resolve --take local|remote [path...]
```

`resolve` compares both sides against their merge base:

- A path changed on one side only is merged automatically (`Auto-merged paths`).
- A path changed differently on both sides needs a decision. Repositories (gitlinks) count as paths.
- `--take local` or `--take remote` decides for all such paths, or only for the paths you name.

```console
$ tasktree resolve --take remote
...
Merged 2 local and 1 remote commit(s); auto-merged: extra.md; took remote: notes.md
Resolved: fd9847819cb5d10131384c97c4a98018e4b2b706
```

`resolve` creates a merge commit (`tasktree(<id>): resolve take remote`), updates the checkout and, if a remote is configured, syncs. Without a divergence it prints `nothing to resolve` and changes nothing. `--json` prints the comparison and the result. `resolve` also needs a clean root.

Repository branches are never reset. If a clean repository is ahead of or diverged from the commit you chose, it keeps its tip. The mismatch is reported, and the root shows that repository as modified until you reconcile it with git inside the repository or record its tip with `tasktree commit`.

## Network use

Most commands never touch the remote. Only these do:

| Calls | Commands | Limits |
|---|---|---|
| **Explicit transfers** | `add`, `open` (cloning and fetching repositories), `push`, `sync`, `resolve`, `setup`, `config set remote` | No wall-clock limit unless you set `sync.timeout`. Stalls are detected. |
| **Implicit, best effort** | `open` pre-fetch of `main` and `task/<id>` when it creates a checkout; the push of `main` and `task/<id>` when a task is closed; the reachability check in `doctor` | At most 10 seconds per git transport command. Failure is a warning and the command continues with local state. |

All other writes, such as `note` and `mark`, stay local until the next `sync`.

Stall detection applies to every transfer. If you have no `GIT_SSH_COMMAND` and no `core.sshCommand`, tasktree runs SSH with `BatchMode=yes`, `ConnectTimeout=15`, `ServerAliveInterval=15` and `ServerAliveCountMax=4`. For HTTP it aborts a transfer slower than 1000 bytes per second for 60 seconds (`http.lowSpeedLimit`, `http.lowSpeedTime`). Prompts are always off (`GIT_TERMINAL_PROMPT=0`), so a command that would ask for a password fails instead.

`sync.timeout` (for example `5m`) adds a wall-clock limit to each git transport command of an explicit transfer. For implicit calls the limit is the smaller of `sync.timeout` and 10 seconds.

## The rewind guard

A tasktree commit records which commit each repository is at. If a repository's HEAD is older than the recorded commit, because a sync skipped the move, committing would revert another machine's work without telling you. tasktree therefore refuses:

- `tasktree commit` refuses unless you pass `--allow-rewind`, and tells you to run `tasktree sync` instead.
- Closing a task refuses unless you pass `mark --force`.

Run `tasktree sync` first. It applies the recorded commit when the repository is clean and on its branch.
