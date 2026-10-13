# Doctor and recovery

`tasktree doctor` checks the tasks repository, the records, every checkout registered on this machine, and the hidden git directories. It reports findings, and with `--fix` it repairs the ones that cannot lose work.

```console
$ tasktree doctor
SEVERITY  CODE              SUBJECT                MESSAGE
ERROR     checkout-missing  8qtbp7                 recorded checkout path does not exist; hidden clone has no work at risk
WARNING   no-remote         /home/alice/.tasktree  no remote configured; ~/.tasktree is the only copy; make a backup
1 errors, 1 warnings, 1 fixable
error: doctor found errors
```

- Findings are sorted errors first, then by code.
- The exit code is 1 while errors remain, otherwise 0. Warnings alone do not fail.
- `--fix` repairs what it can, prints those findings with `(fixed)`, and checks again, so the final counts show what is left.
- `--json` prints `{"findings": [...], "errors": n, "warnings": n}`. See [JSON output](../reference/json-output.md#doctor).

The command needs no network, except one `ls-remote` call that is limited to 10 seconds.

## Codes

| Code | Severity | `--fix` | Meaning and what to do |
|---|---|---|---|
| `check-failed` | error | no | A check itself could not run, for example an unreadable file. The message names the file. Fix the cause and rerun. |
| `no-remote` | warning | no | No remote is configured, so `~/.tasktree` is the only copy. Back it up, or [configure a remote](two-machines.md). |
| `remote-unreachable` | warning | no | `git ls-remote origin` failed or took longer than 10 seconds. Check the network and credentials. |
| `main-ahead` | warning | no | `main` has commits that the last fetched `origin/main` lacks. Run `tasktree sync`. |
| `main-behind` | warning | no | The remote has commits that `main` lacks. Run `tasktree sync`. |
| `hub-interrupted-sync` | error | when the hub has no manual edits | A sync stopped halfway, so the hub index still holds the pre-merge tree. `--fix` restores it. |
| `hub-dirty` | error | no | The tasks repository has uncommitted changes, usually manual edits. Commit or discard them in `~/.tasktree/git`. tasktree refuses to write records meanwhile. |
| `record-unparseable` | error | no | A file in `tasks/` is not a valid record. Fix or restore the file. |
| `conflict-flag` | warning | no | A record has unacknowledged sync conflicts. Use `tasktree info ack <field>` or `info set`. |
| `duplicate-handle` | error | no | Two records share a handle, usually after a sync. Rename one with `tasktree info set handle <new> -t <id>`. |
| `parent-missing` | error | no | A record's `parent` id has no record. Clear it with `tasktree info set parent --clear -t <handle>`. |
| `parent-cycle` | error | no | Parent links form a loop, for example after two machines reparented tasks in opposite directions. Commands that read the task graph, such as `list` and `info`, fail with `task parent cycle detected` until you break it. Break it with `tasktree info set parent --clear -t <id>` on one task in the loop; clearing a parent works even when the task is part of a cycle. |
| `checkout-missing` | error | see below | `local.yml` records a checkout whose directory is gone. |
| `marker-mismatch` | error | no | The directory in `local.yml` has no `.tasktree/config.yml`, or its `taskId` is another task's. Restore the right directory. |
| `path-drift` | error | yes | A repository's `.git` file or `core.worktree` does not point at the checkout's current location, typically after the directory moved. `--fix` rewrites both. |
| `root-core-worktree` | warning | yes | The root's hidden git directory has `core.worktree` set, which it must not. `--fix` unsets it. |
| `branch-unpushed-to-hub` | warning | when ahead | The checkout's HEAD is ahead of the task branch in the hub. `--fix` pushes it. If both moved (diverged), it is not fixable; use `tasktree sync` and `resolve`. |
| `closed-but-checked-out` | warning | no | A closed task still has a checkout. Run `tasktree mark <status> -t <handle>` to remove it through the loss gate. |
| `orphan-clone` | error | see below | A hidden git directory in `~/.tasktree/clones` has no `local.yml` entry. |
| `import-journal` | error | no | A V1 import did not finish. Run `tasktree import --recover <path>`. The hidden git directory of an incomplete import holds the repositories' git data, so `doctor` never reports it as an `orphan-clone` and `--fix` never removes it. |
| `subtask-branch-mismatch` | warning | no | An isolated subtask's branch does not start with its parent's branch name, after a reparent. Informational. |

### `checkout-missing`

| Situation | `--fix` |
|---|---|
| The directory moved: a directory under `tasksRoot` (up to three levels down) or next to the recorded path has a `.tasktree/config.yml` with this task's id | Re-points `local.yml` to that directory and rewrites the repositories' `.git` files and `core.worktree`. Nothing is deleted. |
| The directory is gone, no hidden git directory exists | Removes the `local.yml` entry |
| The directory is gone, a hidden git directory exists, and it holds no unpushed commits or stashes | Removes the hidden git directory and the entry |
| The directory is gone, and the hidden git directory holds unpushed commits or stashes | Not fixable. The message lists them. Restore the directory, or accept the loss with `open --force`. |

### `orphan-clone`

| Situation | `--fix` |
|---|---|
| A directory under `tasksRoot` (up to three levels down) has a matching `.tasktree/config.yml` | Registers that directory in `local.yml` |
| No directory, no unpushed work in the git directory | Removes the hidden git directory |
| No directory, and the git directory holds unpushed commits or stashes | Not fixable. Restore the checkout first. |

## Recipes

### You deleted a checkout directory by hand

Closing a task is the right way to remove a checkout. If you deleted the directory anyway:

```console
$ tasktree doctor --fix
...
ERROR     checkout-missing  s5brtg  recorded checkout path does not exist; hidden clone has no work at risk (fixed)
```

If the finding is not fixable because the hidden clone holds unpushed commits, you can still recover them. The repositories live in `~/.tasktree/clones/<id>.git/modules/<name>`; run `git --git-dir=<that path> log --branches --not --remotes=origin` to see them, and push them from there. When you are finished with them, or want the checkout back as it was last committed, reopen it:

```console
$ tasktree open PROJ-123
error: hidden git dir for PROJ-123 already exists at /home/alice/.tasktree/clones/8qtbp7.git; the checkout may have been moved or local.yml lost
$ tasktree open PROJ-123 --force
/home/alice/work/tasks/PROJ-123
```

`open --force` removes the hidden git directory and builds a fresh checkout from the task branch, so the repositories are at the commits last recorded with `tasktree commit`. It prints what the hidden directory held (unpushed commits, stashes) before it deletes it. Use it only when you do not need that work.

### You moved a checkout directory by hand

The paths are absolute, so a moved directory breaks. The simplest repair:

1. Move the directory back if you can. Nothing else is needed.
2. Otherwise run `tasktree doctor --fix`. It finds the moved directory by its `.tasktree/config.yml` (below `tasksRoot` or next to the old path), re-points `local.yml` to it and rewrites the repository paths. Uncommitted files stay where they are. If you moved it somewhere doctor does not look, edit `~/.tasktree/local.yml`, set `checkouts.<id>.path` to the new directory, then run `tasktree doctor --fix` to repair `path-drift`.

`doctor --fix` deletes a hidden git directory for a missing checkout only when no directory with the task's marker can be found and the repositories hold no unpushed commits or stashes.

Next time, use `tasktree move`, which does all this atomically and verifies the result:

```console
$ tasktree move ~/work/elsewhere/PROJ-123
/home/alice/work/elsewhere/PROJ-123
hint: open shells keep the old $PWD; cd to the new path
```

`move` needs the new path on the same filesystem, empty or missing. It refuses if a repository has a rebase, merge, cherry-pick, revert, bisect or sequencer operation in progress. If anything fails, it moves the directory back and restores the paths.

### A sync was interrupted

`tasktree doctor` reports `hub-interrupted-sync`. If you made no manual edits in `~/.tasktree/git`, `tasktree doctor --fix` repairs it. Then run `tasktree sync` again.

### The hub has manual edits

You may edit `~/.tasktree/git` by hand, for example to fix a record. While it has uncommitted changes, tasktree refuses to write (`hub has uncommitted manual edits`) and doctor reports `hub-dirty`. Commit them there with git, or discard them.

### An import did not finish

Run `import --recover`. `doctor --fix` leaves the import's hidden git directory and journal alone, so it cannot make recovery impossible.

```console
$ tasktree import --recover ~/ws/payments-retry
restored V1 workspace exactly at /home/alice/ws/payments-retry
```

See [Importing V1 workspaces](../getting-started/importing-v1.md#interrupted-imports).

### A repository is empty after `open`

If the commit recorded for a repository is not available (it was never pushed from the other machine), `open` leaves the repository uninitialized and warns. `info` shows state `uninitialized`. Push the commit on the machine that has it, then run `tasktree sync` or `tasktree open` here.
