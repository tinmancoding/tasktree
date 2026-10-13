## tasktree mode

Change a repository mode

### Synopsis

Set the mode of a repository and commit the change. Modes: follow (fast-forward to origin/<branch>), work (fast-forward your branch; never rebase) and pin (never moves). Switching to follow or work records the current branch of the repository; switching to pin removes the recorded branch.

```
tasktree mode <repo> <follow|work|pin> [flags]
```

### Options

```
  -h, --help          help for mode
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

