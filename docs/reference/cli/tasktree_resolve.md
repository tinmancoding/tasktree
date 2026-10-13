## tasktree resolve

Inspect or explicitly merge divergent tasktree histories

### Synopsis

Resolve a task branch that diverged between this machine and the remote.

Without --take, resolve lists the local and remote commits, the paths changed on both sides, and the paths that merge cleanly. It changes nothing when the histories have not diverged.

Paths changed on one side only are merged automatically. Paths changed differently on both sides need a decision: --take local or --take remote applies to all of them, or to the paths given as arguments (repositories count as paths). Resolve then creates a merge commit, updates the checkout and syncs. Repository branches are never reset; if one is ahead of or diverged from the chosen commit, it keeps its tip and the mismatch is reported.

```
tasktree resolve [path...] [flags]
```

### Options

```
  -h, --help          help for resolve
      --json          emit the comparison and result as JSON
      --take string   keep the local or remote version of paths changed on both sides (all such paths unless paths are given)
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

