## tasktree note

Append a note, decision, comment, or ref to a task log

### Synopsis

Append an entry to the task log. The entry records the author, machine and time. The text may not contain reserved headings or a log entry header line.

The author is --author, else TASKTREE_AUTHOR, else the author config key. It must not contain spaces or "@".

```
tasktree note <text> [flags]
```

### Options

```
  -h, --help          help for note
      --kind string   entry kind: note, comment, decision or ref (default "note")
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

