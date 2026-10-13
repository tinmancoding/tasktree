## tasktree add

Add a repository to the tasktree

### Synopsis

Add a repository as a full clone and commit the change. The argument is an alias from aliases.yml or a clone URL. The clone comes from the local cache, which is fetched first. Aliases for the URL are registered automatically.

Branch and mode follow from the flags:

- Neither --branch nor --from: follow the remote default branch (mode follow).
- --branch B, B exists: check out B (mode work).
- --branch B, B is new: create B from --from, else from the default branch (mode work).
- --from F, F is a branch: follow F (mode follow).
- --from F, F is a tag or commit: detached at F (mode pin).

--mode overrides the derived mode. Modes follow and work need a branch.

With -t and no checkout on this machine, the repository is added to the task's definition and cloned on the next open.

```
tasktree add <alias|url> [flags]
```

### Options

```
      --branch string   branch to work on; created if it does not exist
      --from string     branch, tag or commit to start from
  -h, --help            help for add
      --mode string     follow, work or pin (default derived from --branch and --from)
      --name string     directory name (default: last part of the URL)
  -t, --task string     task handle or id
```

### Options inherited from parent commands

```
      --author string   author for log entries (overrides TASKTREE_AUTHOR and the author config key)
  -C, --chdir string    run as if started in this directory
  -v, --verbose         print git commands
```

### SEE ALSO

* [tasktree](tasktree.md)	 - Task management backed by git

