## tasktree info edit

Replace a task description

### Synopsis

Replace the whole description. The new text comes from --file, from --stdin, or from your editor ($VISUAL, then $EDITOR, then vi) on a temporary file holding the current text.

A description may not contain the reserved headings "## Description" or "## Log". If the text is unchanged, nothing is committed.

```
tasktree info edit [flags]
```

### Options

```
      --file string   read the new description from this file
  -h, --help          help for edit
      --stdin         read the new description from standard input
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

