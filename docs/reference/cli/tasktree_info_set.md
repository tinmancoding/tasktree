## tasktree info set

Set a task title, handle, or parent (acknowledging that field's conflict)

### Synopsis

Set one record field: title, handle or parent. A handle must be unique and match [A-Za-z0-9._-], at most 64 characters. A parent is a handle or id; --clear removes it. Reparenting only changes the record; branch names of existing subtask checkouts stay as they are.

Status is not settable here; use "mark".

```
tasktree info set <field> [value] [flags]
```

### Options

```
      --clear         remove the parent (only valid for the parent field)
  -h, --help          help for set
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

