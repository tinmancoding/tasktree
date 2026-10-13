# Subtasks

A subtask is an ordinary task whose record has `parent: <id>`. Like every task it has zero or one tasktree. Depth is unlimited, and the rules below apply at every level.

There are two kinds, which differ only in where the work happens.

| | Shared (default) | Isolated |
|---|---|---|
| Created with | `new --parent P "title"` | `new --parent P --isolated "title"`, or later `open <subtask>` |
| Tasktree | None. Work happens in the parent's tasktree. | Its own, with its own checkout |
| Use it for | Steps of one piece of work, tracked on the record | Work that needs its own branches, reviewed in a separate pull request |

## Shared subtasks

```console
$ tasktree new --parent PROJ-200 "Add retry to client"
add-retry-to-client 4xmk52
```

The new handle is a slug of the title. The subtask is a record, nothing else. Point record commands at it with `-t`, with focus, or with `TASKTREE_TASK`.

### Targeting

Inside a tasktree, a **record command** (`info`, `note`, `mark`, `ref`, `label`, `history`) picks its task in this order:

1. `-t <handle|id>`
2. the `TASKTREE_TASK` environment variable
3. the focused subtask of this checkout
4. the task that owns the tasktree

Every write names its target in its output, so a note on the wrong task is visible at once: `note added to add-retry-to-client`.

**Structural commands** (`add`, `remove`, `mode`, `commit`, `push`, `bootstrap`, `move`) always act on the task that owns the tasktree. Focus and `TASKTREE_TASK` do not retarget them.

### Focus

`tasktree focus <handle>` makes a shared subtask the default target in this checkout. It is stored in `local.yml`, per checkout, so it is not synced.

```console
$ tasktree focus add-retry-to-client
focused add-retry-to-client
$ tasktree note "retry budget = 3"
note added to add-retry-to-client
$ tasktree mark done
add-retry-to-client marked done
$ tasktree focus --clear
focus cleared
```

`info` shows a `Focus:` line while focus is set. Focus must name an open descendant of the checkout's task that has no tasktree of its own; otherwise `focus` exits 6. Closing the focused subtask clears the focus.

When several agents work in one checkout, give each its own `TASKTREE_TASK` instead of sharing the focus:

```console
$ TASKTREE_TASK=PROJ-200 tasktree note "set from the environment"
note added to PROJ-200
```

If `TASKTREE_TASK` names a task that does not exist, the command fails. If it names a shared descendant of the tasktree you are in, the command keeps using that tasktree's root, so `info` still shows the repository table.

## Isolated subtasks

```console
$ tasktree new "Add backoff" --parent PROJ-200 --isolated
add-backoff 6nay2k
/home/alice/work/tasks/add-backoff
```

The parent must be checked out on this machine; otherwise the command exits 6 with `open <parent> first`, before it creates a record. The handle and id go to stderr and the path to stdout, like `new --open`.

`tasktree open <subtask>` on a shared subtask does the same later: it promotes the subtask to an isolated one.

### What the new tasktree contains

- The parent's `work` repositories. `--repos api,web` chooses explicitly and may include `follow` and `pin` repositories. Modes are copied. A name that the parent does not have is an error.
- No root files are copied. A fresh `AGENTS.md` points back to the parent with `tasktree info -t <parent>`.

### Branch names

The branch of each `work` repository is `<parent-branch>-<subtask-handle>`, for example `feature/retry-add-backoff`. `--branch` overrides it. The name is not `feature/retry/add-backoff` because git cannot hold both `feature/retry` and `feature/retry/add-backoff` as branches. `follow` repositories keep the parent's branch. `pin` repositories stay detached.

If the branch already exists on the remote at another commit, or collides with a branch name as a path prefix, the command fails with exit code 5.

### The fork point

The subtask starts from the parent's **live local HEAD**, including commits the parent has not pushed. tasktree adds a remote named `parent` to each repository, pointing at the parent's local git directory, and fetches from it. No push is needed.

- If a parent repository has uncommitted changes, the subtask forks from its last commit and prints a warning.
- If the parent's HEAD is not on the branch recorded in `.gitmodules`, it warns and forks from HEAD.
- To take newer parent work later, use plain git inside the subtask's repository: `git fetch parent`, then merge.

### Returning the work

Isolated work goes back through `origin` only. There is no local merge command.

```console
$ tasktree push
api: pushed feature/retry-add-backoff
gh pr create --repo myorg/api --base feature/retry --head feature/retry-add-backoff
```

`push` sends the `work` repositories and prints a `gh pr create` line with the parent's branch as base. It does not open the pull request. Before it prints the line, it checks that `origin/<parent-branch>` contains the subtask's fork point. If not, it warns on the same line as `pushed`, for example `warning: push PROJ-200's api first, or the PR will include 4 parent commits`.

`push` rules, for every task:

- Without arguments only `work` repositories are pushed. Name a repository to push a `follow` or `pin` one.
- It refuses detached HEAD and the protected branches `main`, `master`, `release/*` and the remote's default branch. All selected repositories are checked before any is pushed.

## Rules for parents and children

- **Closing a parent with open subtasks is refused** (exit 6), and the subtasks are listed. `mark done --cascade` closes them first, deepest first. Each isolated subtask goes through the normal [close gates](tasktrees.md#closing). If any gate refuses, nothing is closed.
- **No automatic rollup.** A parent's status never changes by itself. `list` and `info` show progress as `closed/total` of the direct subtasks, for example `1/2`.
- **Reparenting changes only the record.** `tasktree info set parent <handle> -t <subtask>` or `info set parent --clear` does not rename branches. `doctor` reports `subtask-branch-mismatch` when a subtask's branch no longer starts with its parent's branch. A parent that would create a cycle is rejected.
