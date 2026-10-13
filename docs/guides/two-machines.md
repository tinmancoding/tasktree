# Using two machines

This guide connects two machines, a laptop and a desktop, through a git remote. You will move a task from one to the other, work on both, and see what travels and what does not.

The output is real; names, ids and timestamps are examples.

## What you need

- A private, empty git repository for your task data, for example `git@github.com:myorg/tasks.git`. It holds task records and tasktree branches, not your code.
- tasktree installed on both machines.
- Access to the code repositories from both machines. Repository code travels through its own `origin`, not through the tasks remote.

## 1. First machine: connect the remote

```console
laptop$ tasktree config set remote git@github.com:myorg/tasks.git
```

This adds `origin` to `~/.tasktree/git` and pushes `main` and every `task/*` branch. From now on `tasktree commit` prints a reminder to sync, and the `no-remote` warning from `doctor` goes away.

Create a task and work on it. The code must be pushed before the other machine can use it:

```console
laptop$ tasktree new "Fix login redirect" --jira PROJ-123
PROJ-123 04cr8d
laptop$ tasktree add git@github.com:myorg/api.git --branch fix/PROJ-123 -t PROJ-123
laptop$ cd "$(tasktree open PROJ-123)"
laptop$ (cd api && echo fix > fix.txt && git add . && git commit -m "Fix redirect" && git push -u origin fix/PROJ-123)
laptop$ tasktree commit -m "Record the fix"
committed 4566e0a and pushed to the local hub; run tasktree sync to publish
laptop$ tasktree sync
fetched hub origin
reconciled main and task refs
pushed leased hub refs
Report:
  dirty: none
  diverged: none
  skipped: none
  unpushed gitlinks: none
  checkouts behind: none
  closed-task checkouts: none
  failed: none
```

`commit` records the root and the exact commit of `api` on the task branch. `sync` publishes that to the remote.

## 2. Second machine: join

On the desktop, which has no `~/.tasktree` yet, run `setup` with the remote:

```console
desktop$ tasktree setup --remote git@github.com:myorg/tasks.git --author alice --machine desktop --tasks-root ~/work/tasks
tasktree setup complete
desktop$ tasktree list
HANDLE    STATUS    TITLE               ID      PARENT  PROGRESS  TREE
PROJ-123  incoming  Fix login redirect  04cr8d                    defined
```

`setup` cloned the task data. The task is `defined`: its tasktree exists, with no checkout on this machine. Give each machine its own `--machine` name, because it appears in every log entry.

Open it:

```console
desktop$ tasktree open PROJ-123
/home/alice/work/tasks/PROJ-123
desktop$ cd work/tasks/PROJ-123 && tasktree info
...
Tasktree: /home/alice/work/tasks/PROJ-123
Root changes: 0
NAME  MODE  BRANCH        HEAD          STATE  AHEAD  BEHIND  UNPUSHED
api   work  fix/PROJ-123  3abbf3f1d087  clean  0      0       0
```

`api` is at the same commit you pushed from the laptop, on branch `fix/PROJ-123`. `open` cloned it and set the branch to the commit recorded on the task branch.

## 3. Work on both

Write a note on the desktop and publish it:

```console
desktop$ tasktree note "Reproduced on the desktop"
note added to PROJ-123
desktop$ tasktree sync
```

On the laptop, write another note and sync. The records merge by entry, so no note is lost or duplicated:

```console
laptop$ tasktree note "Added a regression test idea"
laptop$ tasktree sync
laptop$ tasktree info
...
Log:
  2026-10-13T14:25:25Z note alice@desktop: Reproduced on the desktop
  2026-10-13T14:25:26Z note alice@laptop: Added a regression test idea
```

## What travels and what does not

| Travels with `sync` | Does not travel |
|---|---|
| Task records: status, refs, labels, description, log | Uncommitted work in a repository |
| Tasktree definitions: repositories, modes, files, bootstrap steps | Repository commits that you did not push to `origin` |
| Root commits, including the exact commit of every repository | `local.yml`: checkout paths and focus |
| | Files in the root that you did not `tasktree commit` |

A repository commit that is on one machine only is reported on the other as an *unpushed gitlink* (`unpushed gitlinks` in the report), and the repository there stays where it was. Push the code with `tasktree push` or git, then sync again.

## A daily routine

1. `tasktree sync` when you sit down. Outside a tasktree it merges every task and does not touch checkouts.
2. Work. Commit and push code in the repositories. Run `tasktree commit` at checkpoints.
3. `tasktree sync` inside the tasktree before you leave. It also updates `follow` and `work` repositories from `origin` and records the moves.

## When the machines disagree

- **Same field edited on both:** the merge keeps your value, flags the field with `!` in `list`, and keeps the other value in a log comment. Clear it with `tasktree info ack <field>`.
- **Root changed on both:** `sync` stops with exit code 5. Run `tasktree resolve`, then `tasktree resolve --take local|remote`.
- **Dirty root:** `sync` refuses until you `tasktree commit`.
- **Unrelated histories:** if both machines created tasks before the first sync, rerun with `tasktree sync --allow-unrelated`.

[Sync](../concepts/sync.md) explains each case.

## Closing a task on one machine

`tasktree mark done` removes the checkout on the machine where you run it and pushes the closed record to the remote, best effort. On the other machine, `tasktree sync` reports a `closed-task checkout`. Remove it with `tasktree mark done -t PROJ-123`, which applies the normal loss gate there.
