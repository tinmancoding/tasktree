## tasktree history

Show a task record's git history

### Synopsis

Show the commits that changed the task record, newest first: time, git author and subject. This is the record's changelog; there is no separate stored one.

```
tasktree history [flags]
```

### Options

```
  -h, --help          help for history
      --json          emit JSON
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

