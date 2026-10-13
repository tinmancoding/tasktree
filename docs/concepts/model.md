# The model

tasktree has three concepts. Each answers a different question.

| Concept | Question it answers | Where it lives |
|---|---|---|
| **Task** | What is the work, and where does it stand? | A Markdown file `tasks/<id>.md` on the `main` branch of the tasks repository |
| **Tasktree** | Which repositories, files and symlinks make up the workspace? | The branch `task/<id>` of the tasks repository |
| **Checkout** | Where is that workspace on this machine? | A directory, usually `<tasksRoot>/<handle>`, recorded in `local.yml` |

A task has **zero or one tasktree**. Tasks never hold code or repository URLs. A tasktree can exist without a checkout: that is what `tasktree add -t <task>` creates when the task has no checkout on the machine. A checkout is per machine, so the same tasktree can be checked out on two machines at once.

```console
$ tasktree list
HANDLE    STATUS    TITLE               ID      PARENT  PROGRESS  TREE
PROJ-123  active    Fix login redirect  3hawkn                    here
PROJ-124  next      Add rate limit      ng4myp                    defined
PROJ-125  incoming  Update docs         xahs97                    -
```

The `TREE` column shows the state: `here` is a checkout on this machine, `defined` is a tasktree without a checkout here, `-` is a task without a tasktree.

## The tasks repository

`~/.tasktree/git` is an ordinary non-bare git repository, called the *hub* in command output. It is created by `tasktree setup`.

- `main` holds one file per task, `tasks/<id>.md`.
- `task/<id>` is an **orphan** branch, one per tasktree. It never contains `tasks/*.md`. It holds the repositories (as submodules), committed files and symlinks, and `.tasktree/config.yml`.

Every command that changes a record takes a file lock on the hub, edits the file and commits. The lock is released by the kernel if the process dies, so there are no stale locks. Concurrent writers, such as you and two agents, queue for a few milliseconds.

You may edit the hub by hand. tasktree then refuses to write until you commit or discard your edits, and it never includes them in its own commits.

## A checkout

A checkout is a full clone of the hub on `task/<id>`:

```text
~/work/tasks/PROJ-123/            # the tasktree root: not a git repository
  .tasktree/config.yml            # committed marker: kind TasktreeConfig, taskId, bootstrap
  .tasktree/task.md               # generated, ignored by git: a copy of the task record
  .gitmodules                     # repositories, branches and modes
  AGENTS.md                       # starter rules for agents
  api/                            # a full clone: a normal git repository
  web/
```

The root has no `.git`. Its git directory is hidden at `~/.tasktree/clones/<id>.git`. tasktree runs root commands with `GIT_DIR` and `GIT_WORK_TREE` set for that single git process. It never exports them, because inherited values break child processes such as `git clone`. Inside `api/`, git works as usual.

```console
$ git status
fatal: not a git repository (or any of the parent directories): .git
$ cd api && git status -sb
## fix/PROJ-123...origin/fix/PROJ-123
```

### Why the root has no git

Repositories are stored as git submodules, so that a commit on the task branch records the exact commit of every repository. Submodule commands are awkward, easy to get wrong, and not what you want to teach an agent. The rule is therefore simple: **nobody runs git in the root.** Humans and agents use `tasktree` there and git in the repositories.

This also means:

- Tools that look for `.git` to find a project root do not treat the tasktree root as a project. They work inside each repository.
- The root's git state is visible through `tasktree info` (`Root changes`) and changed through `tasktree commit`.

### Repositories are full clones

`add` and `open` clone each repository from a local bare cache, then set `origin` back to the real URL. The clone has its own object store: it does not borrow objects from the cache, so deleting the cache never breaks a checkout. Each repository's git directory lives in `~/.tasktree/clones/<id>.git/modules/<name>`, and the `.git` file inside `api/` points to it with an absolute path.

Because the paths are absolute, **do not move a checkout directory by hand**. Use [`tasktree move`](../reference/cli/tasktree_move.md). If a directory was moved or deleted, [`tasktree doctor`](../guides/doctor-and-recovery.md) finds and repairs the damage.

## Who owns which fact

| Fact | Owner | Never stored in |
|---|---|---|
| Status, title, refs, labels, notes | The record on `main` | The tasktree, `local.yml` |
| Repository URLs, branches, modes | `.gitmodules` on `task/<id>` | The record |
| Exact repository commits | Gitlinks on `task/<id>` | The record |
| Where a checkout is, which subtask is focused | `~/.tasktree/local.yml` | Any git branch |

`local.yml` is never synced. Records and tasktrees travel between machines with [`tasktree sync`](sync.md).

## Rules the tool follows

- **The tool commits what the tool changed; you commit what you changed.** `add`, `remove`, `mode` and `bootstrap add` commit their own change. `commit` records your root edits and the current commit of every repository. tasktree never commits inside a repository.
- **Work is never moved silently.** `sync` skips a dirty repository or a branch that cannot fast-forward, and reports it. It never rebases or force-resets.
- **Code reaches `origin` only through `tasktree push`.** `commit` and `sync` publish the root and the records, never repository code.
- **Destructive steps check for loss first.** `mark done`, `remove` and `open --force` refuse, or list what would be lost, before they delete a clone. See [Closing a tasktree](tasktrees.md#closing).

## Finding the task you mean

Inside a checkout, commands act on the task that owns it. tasktree finds the root by walking up from the current directory to the nearest `.tasktree/config.yml` whose `kind` is `TasktreeConfig`. The machine config `~/.tasktree/config.yml` has a different shape and is skipped.

Anywhere, `-t <handle|id>` names the task. For record commands (`info`, `note`, `mark`, `ref`, `label`, `history`), the order is `-t`, then the `TASKTREE_TASK` environment variable, then the focused subtask of this checkout, then the task that owns the checkout. Structural commands (`add`, `remove`, `mode`, `commit`, `push`, `bootstrap`, `move`, `sync`) ignore focus and `TASKTREE_TASK` and always act on the root's own task. See [Subtasks](subtasks.md).

## What is not regenerable

Everything under `~/.tasktree` except the clone cache is unique data. Without a remote it is the only copy of your records and artifacts. `tasktree doctor` warns about that. See [Files](../reference/files.md) for the full layout.
