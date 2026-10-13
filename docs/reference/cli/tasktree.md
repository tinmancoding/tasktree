## tasktree

Task management backed by git

### Synopsis

Manage tasks and the multi-repository workspaces for them. Git is the only state store.

A task is a Markdown record with a status and a log. A tasktree is the definition of the repositories, files and symlinks that belong to one task. A checkout is a tasktree materialized in a directory on this machine.

Most commands act on the task of the tasktree you are in. Use -t <handle|id> to name a task from anywhere. Inside a tasktree, record commands also honor TASKTREE_TASK and the focused subtask.

Exit codes: 0 success; 1 other error; 2 usage or validation error; 3 not found; 4 not in a tasktree, or the task has no checkout; 5 conflict; 6 refused by a safety gate.

### Options

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -h, --help            help for tasktree
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree add](tasktree_add.md)	 - Add a repository to the tasktree
* [tasktree alias](tasktree_alias.md)	 - Manage repository aliases
* [tasktree bootstrap](tasktree_bootstrap.md)	 - Run, add, remove, or list the tasktree bootstrap steps
* [tasktree commit](tasktree_commit.md)	 - Record root files and repository commits on the task branch
* [tasktree config](tasktree_config.md)	 - Read or change machine config
* [tasktree doctor](tasktree_doctor.md)	 - Check tasktree state and safely repair known problems
* [tasktree focus](tasktree_focus.md)	 - Focus a shared descendant in this tasktree
* [tasktree history](tasktree_history.md)	 - Show a task record's git history
* [tasktree import](tasktree_import.md)	 - Import V1 workspaces without recloning repositories
* [tasktree info](tasktree_info.md)	 - Show a task record and, inside a tasktree, its repositories
* [tasktree label](tasktree_label.md)	 - Manage task labels
* [tasktree list](tasktree_list.md)	 - List tasks (open tasks by default)
* [tasktree mark](tasktree_mark.md)	 - Set task status; closing tears down the local tasktree
* [tasktree mode](tasktree_mode.md)	 - Change a repository mode
* [tasktree move](tasktree_move.md)	 - Relocate a tasktree checkout atomically
* [tasktree new](tasktree_new.md)	 - Create a task (optionally from a template or open its tasktree)
* [tasktree note](tasktree_note.md)	 - Append a note, decision, comment, or ref to a task log
* [tasktree open](tasktree_open.md)	 - Materialize or locate a task's tasktree; prints its path
* [tasktree push](tasktree_push.md)	 - Push repository branches and print PR hints
* [tasktree ref](tasktree_ref.md)	 - Manage typed task references
* [tasktree remove](tasktree_remove.md)	 - Remove a repository from the tasktree
* [tasktree resolve](tasktree_resolve.md)	 - Inspect or explicitly merge divergent tasktree histories
* [tasktree root](tasktree_root.md)	 - Print the tasktree root path
* [tasktree setup](tasktree_setup.md)	 - Initialize ~/.tasktree and machine config
* [tasktree sync](tasktree_sync.md)	 - Synchronize task records, definitions, and repository branches
* [tasktree template](tasktree_template.md)	 - List, show, or validate task templates
* [tasktree version](tasktree_version.md)	 - Print the tasktree version

