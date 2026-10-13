## tasktree list

List tasks (open tasks by default)

### Synopsis

List tasks, oldest first. Without --all or --status, only open tasks (incoming, next, active, blocked) are listed. Subtasks are indented under their parent when the parent is listed too.

The TREE column is "here" when the task has a checkout on this machine, "defined" when it has a tasktree branch but no checkout here, and "-" otherwise. A "!" after the status marks unacknowledged sync conflicts; "info" shows the fields. Acknowledge one with "info ack <field>"; "info set" and "info edit" clear the field they change.

```
tasktree list [flags]
```

### Options

```
      --all               include done and dropped tasks
  -h, --help              help for list
      --json              emit JSON
      --label strings     only tasks with this label (repeatable; all must match)
      --ref stringArray   only tasks with this ref, as <jira|pr|url>:<value> (repeatable; all must match)
      --status strings    only tasks with this status (repeatable; may be a closed status)
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

