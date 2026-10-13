## tasktree info ack

Acknowledge a conflict flag without changing the field

### Synopsis

Remove one field from the record's conflict list. Sync sets the list when both machines changed the same field; the remote value is kept in a comment entry in the log. Exits 3 if the field is not flagged.

```
tasktree info ack <field> [flags]
```

### Options

```
  -h, --help          help for ack
  -t, --task string   task handle or id
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree info](tasktree_info.md)	 - Show a task record and, inside a tasktree, its repositories

