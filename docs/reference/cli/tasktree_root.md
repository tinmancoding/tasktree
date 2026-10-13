## tasktree root

Print the tasktree root path

### Synopsis

Print the checkout directory of the task. Exits 4 if the task has no checkout on this machine.

```
tasktree root [flags]
```

### Options

```
  -h, --help          help for root
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

