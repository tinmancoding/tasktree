## tasktree focus

Focus a shared descendant in this tasktree

### Synopsis

Make record commands (info, note, ref, label, mark, history) in this checkout target a shared subtask instead of the task that owns the tasktree. Structural commands are not affected. Focus is stored per checkout in local.yml and clears when the subtask is closed. Use TASKTREE_TASK instead when several agents share one checkout.

```
tasktree focus [handle|id] [flags]
```

### Options

```
      --clear   clear the focus
  -h, --help    help for focus
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

