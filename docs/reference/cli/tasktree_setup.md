## tasktree setup

Initialize ~/.tasktree and machine config

### Synopsis

Create the tasks repository in ~/.tasktree (or $TASKTREE_HOME) and write config.yml. Safe to repeat: only the flags you pass change the config.

With --remote on a machine that has no tasks repository, setup clones it from the remote. With --remote on an existing repository it behaves like "config set remote".

When aliases.yml does not exist yet, setup copies V1 repository aliases into it if it finds them.

```
tasktree setup [flags]
```

### Options

```
      --author string       author name for log entries
  -h, --help                help for setup
      --machine string      machine name for log entries (default: hostname)
      --remote string       URL of the tasks remote
      --tasks-root string   directory for new checkouts (default ~/Developer/tasks)
```

### Options inherited from parent commands

```
  -C, --chdir string   run as if started in this directory
  -v, --verbose        print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

