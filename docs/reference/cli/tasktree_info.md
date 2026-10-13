## tasktree info

Show a task record and, inside a tasktree, its repositories

### Synopsis

Show the task record: fields, refs, subtasks, description and log. For a task with a checkout, also show the root's uncommitted changes and a table of its repositories (mode, branch, HEAD, state, ahead/behind, unpushed). For a task with a definition but no checkout here, show the defined repositories and their recorded commits.

The task is -t, else TASKTREE_TASK, else the focused subtask, else the task of the tasktree you are in.

With --json the output is one JSON object; see the JSON output reference.

```
tasktree info [flags]
```

### Options

```
  -h, --help          help for info
      --json          emit JSON
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
* [tasktree info ack](tasktree_info_ack.md)	 - Acknowledge a conflict flag without changing the field
* [tasktree info edit](tasktree_info_edit.md)	 - Replace a task description
* [tasktree info set](tasktree_info_set.md)	 - Set a task title, handle, or parent (acknowledging that field's conflict)

