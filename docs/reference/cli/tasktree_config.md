## tasktree config

Read or change machine config

### Synopsis

Read or change ~/.tasktree/config.yml.

Keys: remote, tasksRoot, author, machine, jira.baseUrl, sync.jobs, sync.timeout.

Setting remote adds or replaces origin on the tasks repository and pushes main and every task branch. Setting it to "" removes origin.

```
tasktree config [flags]
```

### Options

```
  -h, --help   help for config
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git
* [tasktree config get](tasktree_config_get.md)	 - Print a config value
* [tasktree config set](tasktree_config_set.md)	 - Set a config value

