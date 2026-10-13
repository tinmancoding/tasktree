## tasktree move

Relocate a tasktree checkout atomically

### Synopsis

Move a checkout to a new path on the same filesystem, rewrite the repository paths git keeps, update local.yml and verify that every repository is unchanged. On any failure the move is rolled back. The target must not exist or must be empty. Shells that sit in the old directory keep the old path; cd to the new one.

```
tasktree move <new-path> [flags]
```

### Options

```
  -h, --help          help for move
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

