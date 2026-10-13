## tasktree import

Import V1 workspaces without recloning repositories

```
tasktree import <v1-path> [flags]
```

### Options

```
      --all             import all entries from the V1 registry
      --allow-large     allow committing root files larger than 50 MB
      --dry-run         print the plan without changing anything
      --handle string   V2 task handle
  -h, --help            help for import
      --move            atomically move under tasksRoot after import
      --recover         recover an interrupted import from its journal
      --status string   V2 task status (default active)
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

