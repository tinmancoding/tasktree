## tasktree remove

Remove a repository from the tasktree

### Synopsis

Delete the repository directory and its hidden git directory, and commit the removal. Refuses when the repository has uncommitted changes, commits not on origin, or a stash; --force lists them and proceeds. With -t and no checkout here, only the definition changes.

```
tasktree remove <name> [flags]
```

### Options

```
      --force         remove even if work would be lost; the loss is listed on stderr
  -h, --help          help for remove
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

