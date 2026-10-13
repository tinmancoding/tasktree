## tasktree new

Create a task (optionally from a template or open its tasktree)

### Synopsis

Create a task record and print "<handle> <id>". The handle is --handle, else the --jira key, else a slug of the title.

A task starts without a tasktree. Add repositories with "add -t <handle>", or create the task from a template. With --open, the tasktree is created and checked out, and its path is printed.

With --template, the title is optional and k=v arguments set template parameters. --jira, --handle, --status and the title override the template values.

With --parent, the new task is a subtask of the parent. It shares the parent's tasktree unless you pass --isolated, which creates a tasktree of its own with the parent's work repositories.

```
tasktree new [title] [k=v ...] [flags]
```

### Options

```
      --branch string     branch for the subtask's work repositories (default <parent-branch>-<subtask-handle>)
      --handle string     task handle (default: Jira key, else slug of the title)
  -h, --help              help for new
      --isolated          give the subtask its own tasktree (requires --parent)
      --jira string       Jira key; becomes the handle and a jira ref
      --open              also create and check out a tasktree; prints its path
      --parent string     make this a subtask of a task (handle or id)
      --path string       checkout path (default <tasksRoot>/<handle>)
      --repos strings     parent repositories to copy into an isolated subtask (default: the parent's work repositories)
      --status string     initial status: incoming, next, active, blocked, done or dropped (default "incoming")
      --template string   create from a template name or file
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

