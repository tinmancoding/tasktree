## tasktree open

Materialize or locate a task's tasktree; prints its path

### Synopsis

Print the path of the task's checkout, creating it first when this machine has none.

If a remote is configured and the task has no checkout here, open first fetches main and the task branch, best effort. The checkout is created at <tasksRoot>/<handle> unless --path is given. Repositories are cloned from the local cache, attached to their branches, and bootstrap steps run unless --skip-bootstrap is set. A repository whose recorded commit is not available yet is left uninitialized and reported; sync or the next open retries it.

If the task already has a checkout here, open only prints its path. A closed task cannot be opened. A subtask without its own tasktree becomes an isolated subtask: it gets a tasktree with the parent's work repositories (or --repos), on branch <parent-branch>-<subtask-handle> (or --branch). The parent must be checked out.

```
tasktree open <handle|id> [flags]
```

### Options

```
      --branch string    branch for the work repositories when promoting a subtask
      --force            replace an existing hidden git directory for this task; work in it is listed first
  -h, --help             help for open
      --path string      checkout path (default <tasksRoot>/<handle>)
      --repos strings    parent repositories to copy when promoting a subtask
      --skip-bootstrap   do not run the bootstrap steps
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

