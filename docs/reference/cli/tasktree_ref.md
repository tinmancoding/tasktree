## tasktree ref

Manage typed task references

### Synopsis

Add or remove references to external items. Types: jira (a key such as PROJ-123), pr (owner/repo#N) and url. References are stored on the record; tasktree never calls the external system.

```
tasktree ref [flags]
```

### Options

```
  -h, --help   help for ref
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git
* [tasktree ref add](tasktree_ref_add.md)	 - Add a task reference (types: jira, pr, url)
* [tasktree ref rm](tasktree_ref_rm.md)	 - Remove a task reference (exits 3 if absent)

