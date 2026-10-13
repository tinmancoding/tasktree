## tasktree commit

Record root files and repository commits on the task branch

### Synopsis

Commit everything in the tasktree root (files, symlinks and the current commit of every repository) on the task branch, and push it to the local tasks repository. Prints "nothing to commit" when the root is clean. Never commits inside repositories; repositories with uncommitted changes produce a warning.

Refuses to record a repository commit that is older than the one already recorded, unless --allow-rewind is set.

```
tasktree commit [flags]
```

### Options

```
      --allow-rewind     allow recording a repository commit older than the one already recorded
  -h, --help             help for commit
  -m, --message string   commit message (default: tasktree(<id>): checkpoint)
  -t, --task string      task handle or id
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

