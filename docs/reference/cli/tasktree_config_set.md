## tasktree config set

Set a config value

### Synopsis

Set one config key. sync.jobs must be a positive integer. sync.timeout must be a positive duration such as 60s. See "config" for the keys.

```
tasktree config set <key> <value> [flags]
```

### Options

```
  -h, --help   help for set
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree config](tasktree_config.md)	 - Read or change machine config

