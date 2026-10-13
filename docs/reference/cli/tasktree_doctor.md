## tasktree doctor

Check tasktree state and safely repair known problems

### Synopsis

Check the tasks repository, records, checkouts and hidden git directories. Each finding has a code, a severity (error or warning) and a flag saying whether --fix can repair it.

--fix applies only repairs that cannot discard work, then checks again. Exits 1 when errors remain; warnings alone exit 0.

```
tasktree doctor [flags]
```

### Options

```
      --fix    apply the repairs that cannot lose work, then check again
  -h, --help   help for doctor
      --json   emit JSON
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

