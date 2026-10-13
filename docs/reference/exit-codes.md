# Exit codes

tasktree exits with one of seven codes. Error messages go to standard error, prefixed with `error: `.

| Code | Name | Meaning |
|---|---|---|
| 0 | Success | The command did what you asked. |
| 1 | Error | Any failure that has no more specific code. |
| 2 | Usage | Wrong arguments, an unknown command or flag, or an invalid value. |
| 3 | Not found | The task, repository, alias, step, reference, field or template does not exist. |
| 4 | Not in a tasktree | The command needs a tasktree and there is none to act on. |
| 5 | Conflict | The request clashes with existing state, or machines disagree. |
| 6 | Gate refused | A safety check stopped the command to protect your work. |

## 0: success

- Commands that change nothing because there is nothing to do also exit 0: `commit` with `nothing to commit`, `ref add` of an existing reference, `resolve` with `nothing to resolve`, `info edit` with an unchanged text, `import` of an already imported workspace.
- `doctor` exits 0 when it finds only warnings.
- `import --all` exits 0 when every workspace was imported or skipped.

## 1: error

- Git failed: a clone, fetch or push was rejected or timed out, a path is unreadable, a rebase marker is in the way.
- `tasktree is not set up; run tasktree setup`.
- `doctor` finds errors that remain after `--fix`.
- `sync` or `resolve` finished with failed steps (`some sync steps failed`), for example a repository that could not fetch.
- A bootstrap step failed (`bootstrap step "deps" failed with exit code 1`).
- A destination already exists (`add`, `open --path`, `import --move`).
- No author is configured when a log entry needs one.
- A task graph error: `task parent cycle detected`.
- `import` failed operationally, or `import --all` stopped after a failure.
- A cascade stopped halfway: the message lists what was closed and what remains.
- The close completed, but removing the directory did not (`teardown incomplete`).

## 2: usage

- A wrong number of arguments (`tasktree note accepts 1 arg(s), received 0`).
- An unknown command, flag or shorthand, a flag without its argument, or a flag value of the wrong type.
- An invalid value: status, kind, label, handle, ref type or value, mode, config key, `sync.jobs`, `sync.timeout`, `--take`.
- Conflicting options: `--isolated` with `--template`, `--repos` or `--branch` without `--isolated`, `--open` with `--isolated`, `--handle` with `--all`.
- A description or note that contains a reserved heading.
- A template that fails validation, a template parameter that is missing or undeclared, or an unresolved variable.
- `info set parent` that would make a task its own ancestor.
- `open <subtask> --repos x` where the parent has no repository `x`.

## 3: not found

- No task has the given handle or id (`task "x" not found`).
- A repository name that the tasktree does not have (`remove`, `mode`, `push`).
- An alias, a bootstrap step, a reference to remove, or a conflict field to acknowledge that does not exist.
- A template name or file that does not exist.

## 4: not in a tasktree

- The current directory is not inside a tasktree and `-t` was not given: `not in a tasktree; pass -t`. This applies to `info`, `note`, `mark`, `commit`, `focus` and the other commands that need a task or a checkout.
- The command acts on a checkout and the task has none on this machine: `PROJ-123 has no checkout on this machine; run tasktree open PROJ-123`. This applies to `root`, `commit`, `push`, `move`, `bootstrap`, `sync -t` and `resolve -t`.

## 5: conflict

- A handle already exists (`new`, `info set handle`, a template), or a handle given to a command is ambiguous because two records share it.
- An alias is already used for another URL.
- `sync` refuses: the root has uncommitted changes, `task/<id>` diverged, the push lease was rejected three times, `main` histories are unrelated (use `--allow-unrelated`), or a non-task file on `main` conflicts.
- `resolve` needs a decision: paths changed on both sides need `--take`, or the three histories have no common base.
- An isolated subtask's branch name exists at another commit, or collides with an existing branch as a path prefix.
- The tasks repository is busy or inconsistent: the write lock timed out, it has uncommitted manual edits, an interrupted sync has manual edits, or a ref changed during an operation.

## 6: gate refused

A gate protects work that would be lost, or an operation that would be unsafe. Read the message; it lists the cause.

- `mark done|dropped` and `mark --cascade`: the task has open subtasks, a repository has uncommitted changes, unpushed commits or a stash, the root has ignored files or an undeclared git repository, or a repository's HEAD is older than the recorded commit. `--force` overrides the loss and rewind checks, not the open subtasks check; `--cascade` handles that.
- `remove`: the repository has uncommitted changes, unpushed commits or a stash. `--force` overrides.
- `commit`: a repository's HEAD is older than the recorded commit. `--allow-rewind` overrides.
- `open`: the task is closed, or a hidden git directory for the task already exists (`--force` replaces it), or an isolated subtask needs its parent checked out first.
- `focus`: the target is closed, is not a descendant of the checkout's task, or has its own tasktree.
- `push`: a protected branch or a detached HEAD in a selected repository, or a repository that is not initialized yet.
- `move`: the recorded directory is gone, the target is inside the checkout or not empty, the target is on a different filesystem, or a repository has an operation in progress.
- `import`: the plan has refusals, an incomplete import exists for the folder, or `--all` refused at least one workspace.
