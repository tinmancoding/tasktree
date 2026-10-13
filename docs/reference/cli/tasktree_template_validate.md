## tasktree template validate

Validate a template

### Synopsis

Check apiVersion, kind, metadata.name and that every {{variable}} is declared. The argument is a template name or a path to a file. Exits 2 when invalid, 3 when not found.

```
tasktree template validate <name|file> [flags]
```

### Options

```
  -h, --help   help for validate
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree template](tasktree_template.md)	 - List, show, or validate task templates

