## tasktree mark

Set task status; closing tears down the local tasktree

### Synopsis

Set the status to incoming, next, active, blocked, done or dropped. This is the only command that changes status.

Closing a task (done or dropped) that has a checkout on this machine runs the close flow:
  1. Refuse if the task has open subtasks (see --cascade).
  2. Refuse if work would be lost: uncommitted changes, commits not on origin or a stash in any repository, ignored files or nested git repositories in the root, or a repository whose HEAD is older than the commit recorded for it (see --force).
  3. Commit the root as "close: <status>" and push the task branch to the local tasks repository.
  4. Record the status, then push main and the task branch to the remote, best effort, if one is configured.
  5. Delete the checkout directory, its hidden git directory and its local.yml entry.
The task branch stays, so the task can be reopened later.

Closing a task without a checkout only changes the record.

```
tasktree mark <status> [flags]
```

### Options

```
      --cascade       close all open subtasks first, deepest first; all gates are checked before any change
      --force         close even if work would be lost; the loss is listed on stderr
  -h, --help          help for mark
      --keep          change status only; keep the checkout
  -t, --task string   task handle or id
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

