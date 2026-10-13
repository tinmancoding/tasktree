## tasktree config get

Print a config value

### Synopsis

Print one config value. Keys: remote, tasksRoot, author, machine, jira.baseUrl, sync.jobs, sync.timeout. Unset values print an empty line.

```
tasktree config get <key> [flags]
```

### Options

```
  -h, --help   help for get
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree config](tasktree_config.md)	 - Read or change machine config

