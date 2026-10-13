## tasktree push

Push repository branches and print PR hints

### Synopsis

Push repository branches to origin. Without arguments, only repositories in work mode are pushed. Name follow or pin repositories to push them too. All selected repositories are checked before any push: a detached HEAD, and the branches main, master, release/* and the remote default branch are refused.

For an isolated subtask, push checks that the parent's branch on origin contains the subtask's fork point, warns if not, and prints a "gh pr create" line with the parent branch as base. It never opens a pull request.

```
tasktree push [repo...] [flags]
```

### Options

```
  -h, --help          help for push
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

