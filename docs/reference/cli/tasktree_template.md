## tasktree template

List, show, or validate task templates

### Synopsis

Templates create a task and, optionally, its tasktree. They come from ~/.tasktree/templates and from the built-in set (bugfix, feature, investigation). A template in your directory replaces a built-in one with the same name.

```
tasktree template [flags]
```

### Options

```
  -h, --help   help for template
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git
* [tasktree template list](tasktree_template_list.md)	 - List available templates
* [tasktree template show](tasktree_template_show.md)	 - Show template parameters and raw YAML
* [tasktree template validate](tasktree_template_validate.md)	 - Validate a template

