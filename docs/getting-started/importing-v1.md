# Importing V1 workspaces

tasktree V1 kept a workspace as a folder with a `Tasktree.yml` file and one clone per source. `tasktree import` turns such a folder into a V2 task and tasktree **in place**. It does not reclone anything. Branches, unpushed commits, uncommitted changes and stashes stay exactly as they are.

The V1 documentation stays on the [`v1` branch](https://github.com/tinmancoding/tasktree/tree/v1/docs).

## Before you start

1. **Keep a V1 binary.** `make install` replaces `tasktree` and `tt`. You need the V1 binary once more, to clean its registry after the import. Copy it first:

    ```bash
    cp "$(command -v tasktree)" ~/.local/bin/tasktree-v1
    ```

2. **Back up your workspaces.** Import verifies every repository before and after and rolls back on any difference, but a backup costs little. On macOS, `cp -cR` makes a cheap copy-on-write copy:

    ```bash
    cp -cR ~/ws/payments-retry ~/ws-backup/payments-retry
    ```

3. **Install V2 and run setup:**

    ```bash
    tasktree setup --author alice --machine laptop
    ```

    `setup` also copies your V1 repository aliases from `~/.config/tasktree/repos.yml` to `~/.tasktree/aliases.yml`, if `aliases.yml` does not exist yet.

4. **Commit or push what matters in the repositories.** Import accepts dirty repositories and keeps them dirty, but pushed work is the safest work.

## Step 1: dry run

`--dry-run` prints the plan and changes nothing:

```console
$ tasktree import --dry-run ~/ws/payments-retry
handle: payments-retry
path: /home/alice/ws/payments-retry
target: /home/alice/ws/payments-retry
source api: path=api type=git branch=feature/retry mode=work dirty=0 stashes=0 unpushed=0
root files: 1 (6 bytes)
dry-run: no changes
```

Read it for each source: the branch, the mode tasktree will assign, and the counts of uncommitted files, stashes and unpushed commits. Any `refusal:` line means the import would not run. See [Refusals](#refusals).

## Step 2: import one small workspace

```console
$ tasktree import ~/ws/payments-retry
handle: payments-retry
path: /home/alice/ws/payments-retry
target: /home/alice/ws/payments-retry
source api: path=api type=git branch=feature/retry mode=work dirty=0 stashes=0 unpushed=0
root files: 1 (6 bytes)
imported /home/alice/ws/payments-retry as payments-retry (qppf31)
V1 registry not modified; remove the entry with V1's tasktree prune or delete ~/.local/state/tasktree/registry.toml
```

Check the result before you import more:

```bash
tasktree info -t payments-retry
git -C ~/ws/payments-retry/api status
```

The folder stays where it is. Pass `--move` to relocate it to `<tasksRoot>/<handle>` in the same step. Pass `--handle` to choose another handle than the folder name, and `--status` to start in a status other than `active`.

## Step 3: import the rest

`--all` imports every workspace listed in the V1 registry, `~/.local/state/tasktree/registry.toml` (override the path with `TASKTREE_V1_REGISTRY`):

```console
$ tasktree import --all
...
/home/alice/old/gone: skipped: path missing
summary: imported=1 refused=0 failed=0 skipped=2
```

- A path that no longer exists, or is not a V1 workspace, is skipped.
- A workspace that is already imported is skipped and reported as `already imported`.
- A refused workspace is reported and the run continues. The final exit code is 6.
- An operational failure stops the run at once, because it may have left a journal behind. The exit code is 1.

`--handle` and `--status` cannot be combined with `--all`.

## What import does

| V1 | V2 |
|---|---|
| Folder name | Task handle (or `--handle`) |
| `metadata.name` | Title |
| `metadata.description` | Description |
| `metadata.createdAt` | `createdAt` |
| `metadata.labels` | Label names (the values are dropped) |
| Annotations `jira`, `ticket` or `jira.ticket` that look like `ABC-123` | `jira` ref |
| Other annotation values that start with `http://` or `https://` | `url` refs |
| The whole `Tasktree.yml` | Quoted in the first log entry, `Imported from V1` |
| Each `git` source | A repository in the tasktree; its `.git` directory moves into `~/.tasktree/clones/` and is not copied |
| Files in the folder, including symlinks | Committed on the task branch as they are |

The new task has status `active` unless you pass `--status`.

**Branch and mode come from each checkout's current state, not from the V1 file:**

| Checkout state | Mode |
|---|---|
| On the default branch of `origin` | `follow` |
| On another branch | `work` |
| Detached HEAD | `pin` |

If `origin` has no default branch, the mode is `work` and the plan says so.

The importer also writes a starter `AGENTS.md` if the folder has none, and a `.tasktree/config.yml` marker. It then verifies, for every repository, that HEAD, branch, status, stashes, refs and git config are unchanged, and that the root's committed state matches. Any difference rolls the whole import back and restores `Tasktree.yml`.

**Not converted:** sources of type `local`, `static`, `http` and `archive` are not tracked as sources any more. Their files and symlinks are committed as plain files. Symlink targets are stored unchanged, so an absolute target works on this machine only.

## Refusals

Import refuses a workspace, and exits 6, when any of these hold. The plan lists each one on a `refusal:` line.

- The folder has a legacy `.tasktree.toml` and no `Tasktree.yml`. Run `tasktree migrate` with the V1 binary first.
- The handle is not valid (`[A-Za-z0-9._-]`, at most 64 characters), or another task already has it. Pass `--handle`.
- A git source is a symlink, has no standalone `.git` directory, has submodules, linked worktrees or no commits, or has no `origin` remote.
- A git source has a rebase, merge, cherry-pick, revert, bisect or sequencer operation in progress, or a stale `index.lock`.
- The folder contains a git repository that `Tasktree.yml` does not declare.
- A root file is larger than 50 MB, or the root files total more than 200 MB. Pass `--allow-large` to accept them.
- The workspace and `~/.tasktree` are on different filesystems. Import relies on atomic renames.
- With `--move`, the target `<tasksRoot>/<handle>` exists and is not empty.

A declared git source that is missing from disk is not a refusal. It produces a warning and is left out.

## Interrupted imports

Import keeps a journal in `~/.tasktree/imports/<id>/` until it finishes. If the process dies, `import` and `doctor` report an incomplete import. Recover it with:

```bash
tasktree import --recover ~/ws/payments-retry
```

`doctor --fix` never touches the hidden git directory of an incomplete import. `--recover` takes exactly one path. If the task record was not created yet, it restores the V1 workspace exactly as it was. If the record exists, it repairs `local.yml` and removes the journal. See [Doctor and recovery](../guides/doctor-and-recovery.md).

## Clean up the V1 registry

Import never edits the V1 registry, so imported folders stay listed there. The V2 binary has no `prune` command, and the `tasktree prune` named in the import message is the V1 command. There are two ways to clean up:

- Run V1's `tasktree prune` after importing, while the V1 binary is still installed (for example when you built V2 into another directory instead of replacing V1). The imported folder no longer contains a `Tasktree.yml`, so V1 drops its entry. Run it before you install V2 over V1.
- After you have imported everything, delete the V1 registry file, `~/.local/state/tasktree/registry.toml`. Nothing in V2 reads it except `import --all`.

## Do not run the V1 binary on imported folders

An imported folder is a V2 tasktree. Use `tasktree` for everything at its root. Inside the repositories, git works as before.
