# Quickstart

This walkthrough takes one bug fix from a new task to a closed one. It uses a repository `myorg/api` that you can push to. Replace the URL with one of your own.

Output shown here is real. Task IDs, timestamps and commit hashes differ on your machine.

## 1. Set up

```console
$ tasktree setup --author alice --machine laptop --tasks-root ~/work/tasks
tasktree setup complete
warning: ~/.tasktree is the only copy; back it up
```

`setup` creates `~/.tasktree` and writes `config.yml`. The author and machine name appear in every log entry you write. `--tasks-root` is the directory where checkouts are created; the default is `~/Developer/tasks`. The warning goes away when you configure a [remote](../guides/two-machines.md).

## 2. Create a task

```console
$ tasktree new "Fix login redirect" --jira PROJ-123
PROJ-123 c9sezd
```

`new` prints the handle and the ID. The handle is the Jira key when you give one, otherwise a slug of the title. A new task has a record and nothing else: no directory and no repositories.

## 3. Add a repository

```console
$ tasktree add git@github.com:myorg/api.git --branch fix/PROJ-123 -t PROJ-123
registered alias api
registered alias myorg-api
added api (work)
```

`-t PROJ-123` names the task, because you are not inside a tasktree yet. The repository is added to the task's definition. `--branch` makes it a `work` repository on a branch of your own, created from the default branch if it does not exist. tasktree also registered two aliases, so `tasktree add api` works from now on.

```console
$ tasktree list
HANDLE    STATUS    TITLE               ID      PARENT  PROGRESS  TREE
PROJ-123  incoming  Fix login redirect  c9sezd                    defined
```

`defined` in the `TREE` column means the task has a tasktree but no checkout on this machine.

## 4. Open it

```console
$ tasktree open PROJ-123
/home/alice/work/tasks/PROJ-123
```

`open` clones each repository and prints the directory. Most people jump straight in:

```bash
cd "$(tasktree open PROJ-123)"
```

```console
$ ls -A
.gitmodules
.tasktree
AGENTS.md
api
```

`api/` is a normal git clone, on `fix/PROJ-123`. `AGENTS.md` is a starter file with rules for coding agents. The directory itself is not a git repository, on purpose:

```console
$ git status
fatal: not a git repository (or any of the parent directories): .git
```

Use plain git inside `api/`. Use `tasktree` for everything at the root.

## 5. Work

```console
$ cd api
$ echo retry > fix.txt
$ git add fix.txt
$ git commit -m "Fix redirect loop"
$ git push -u origin fix/PROJ-123
```

Back in the root, `info` shows the task and a table of its repositories:

```console
$ cd ..
$ tasktree info
ID: c9sezd
Handle: PROJ-123
Title: Fix login redirect
Status: incoming
Parent: -
Labels: 
Created: 2026-10-13T14:29:01Z
Updated: 2026-10-13T14:29:01Z
Refs:
  jira: PROJ-123
Description:

Log:
Tasktree: /home/alice/work/tasks/PROJ-123
Root changes: 1
NAME  MODE  BRANCH        HEAD          STATE  AHEAD  BEHIND  UNPUSHED
api   work  fix/PROJ-123  1b74d637e06a  clean  0      0       0
```

`Root changes: 1` is the new commit in `api`: the root records which commit each repository is at.

## 6. Commit the root

```console
$ tasktree commit -m "Record the redirect fix"
committed 08d1853 and pushed to the local hub
```

`commit` records the root files and the current commit of every repository on the task branch. It never commits inside `api/`. That was your job in step 5.

## 7. Write down what you learned

```console
$ tasktree note "Root cause: the return URL was decoded twice"
note added to PROJ-123
$ tasktree note "Keep the existing URL helper" --kind decision
note added to PROJ-123
```

Notes go into the task's log with your author name, the machine and a timestamp. Kinds are `note`, `comment`, `decision` and `ref`.

## 8. Mark it done

```console
$ tasktree mark done
tasktree removed; run: cd ..
PROJ-123 marked done
```

Closing a task removes its checkout, but first it checks that nothing would be lost. If `api/` had uncommitted changes or commits that are not on `origin`, `mark done` would refuse and list them. See [Closing a tasktree](../concepts/tasktrees.md#closing).

The task and its tasktree branch stay. You can read the history any time:

```console
$ cd ..
$ tasktree list --all
HANDLE    STATUS  TITLE               ID      PARENT  PROGRESS  TREE
PROJ-123  done    Fix login redirect  c9sezd                    defined
$ tasktree history -t PROJ-123
2026-10-13T14:29:03Z  alice  task(c9sezd): status incoming → done
2026-10-13T14:29:03Z  alice  task(c9sezd): decision added
2026-10-13T14:29:03Z  alice  task(c9sezd): note added
2026-10-13T14:29:01Z  alice  task(c9sezd): create "Fix login redirect"
```

## Next

- [The model](../concepts/model.md) explains what is on disk and why the root has no `.git`.
- [Tasktrees](../concepts/tasktrees.md) covers repository modes and the close flow.
- [Two machines](../guides/two-machines.md) shows how to sync through a git remote.
