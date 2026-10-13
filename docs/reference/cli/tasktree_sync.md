## tasktree sync

Synchronize task records, definitions, and repository branches

### Synopsis

Exchange state with the tasks remote. Without a remote, sync still reconciles the local tasks repository and reports local state.

Inside a tasktree (or with -t), sync does the following:
  1. Fetch the remote.
  2. Merge main. Task records are merged field by field: log entries are united in time order, labels and refs are merged as sets, and a field changed on both sides keeps the local value, gets a "comment" log entry with the remote value, and is flagged as a conflict (shown as "!" in list; clear it with "info ack").
  3. Fast-forward the task branch. If local and remote both moved, sync stops with exit 5; run "resolve".
  4. Apply recorded repository commits that came from the other machine to clean repositories on their branch.
  5. Update repositories by mode, up to sync.jobs at a time: follow and work repositories fast-forward to origin; dirty or diverged ones are skipped and reported. Gitlink moves this causes are committed.
  6. Push main and the task branch with a lease. A rejected push repeats the sync, up to 3 times.
  7. Report dirty, diverged, skipped and unpushed state.

Sync refuses to run when the root has uncommitted changes; run "commit" first. It never pushes code to a repository's origin; use "push".

Outside a tasktree, sync is the same as --all: it exchanges main and every task branch and does not touch any checkout. It reports checkouts that are behind and checkouts of closed tasks.

```
tasktree sync [flags]
```

### Options

```
      --all               exchange main and every task branch; do not touch checkouts
      --allow-unrelated   allow the first merge of main histories that share no commit
  -h, --help              help for sync
      --json              emit the report as JSON
      --no-repos          skip step 5 (updating repositories from origin); recorded commits from the other machine are still applied
  -t, --task string       task handle or id
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

