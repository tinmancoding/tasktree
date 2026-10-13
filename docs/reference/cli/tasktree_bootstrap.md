## tasktree bootstrap

Run, add, remove, or list the tasktree bootstrap steps

### Synopsis

Bootstrap steps are shell commands stored in .tasktree/config.yml. They run in order, stop at the first failure, and run on "open" when a checkout is created. Without a subcommand, "bootstrap" runs them now. Each step gets TASKTREE_ROOT set to the tasktree root.

```
tasktree bootstrap [flags]
```

### Options

```
  -h, --help          help for bootstrap
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
* [tasktree bootstrap add](tasktree_bootstrap_add.md)	 - Add a bootstrap step and commit it
* [tasktree bootstrap list](tasktree_bootstrap_list.md)	 - List bootstrap steps
* [tasktree bootstrap rm](tasktree_bootstrap_rm.md)	 - Remove a bootstrap step and commit the change
* [tasktree bootstrap run](tasktree_bootstrap_run.md)	 - Run the tasktree bootstrap steps

