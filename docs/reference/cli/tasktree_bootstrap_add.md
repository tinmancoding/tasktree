## tasktree bootstrap add

Add a bootstrap step and commit it

### Synopsis

Append a step. The name must be unique. The command runs with sh -c.

```
tasktree bootstrap add <name> <run> [flags]
```

### Options

```
      --env stringArray   environment variable KEY=VALUE (repeatable)
  -h, --help              help for add
      --workdir string    directory to run in, relative to the tasktree root (default: the root)
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -t, --task string     task handle or id
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree bootstrap](tasktree_bootstrap.md)	 - Run, add, remove, or list the tasktree bootstrap steps

