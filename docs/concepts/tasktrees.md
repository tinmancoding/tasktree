# Tasktrees

A tasktree is the definition of a task's workspace. It is stored on the orphan branch `task/<id>` of the tasks repository and contains only:

- repositories, as git submodules (a `.gitmodules` entry plus a gitlink, which is the exact commit),
- committed files and symlinks,
- `.tasktree/config.yml`, the marker and the bootstrap steps.

There is no `Tasktree.yml` as in V1. `.gitmodules` is the repository list, and the gitlinks are the exact state.

## `.gitmodules`

Each repository is one section. tasktree adds a `tasktree-mode` key, which git ignores:

```ini
[submodule "api"]
	path = api
	url = git@github.com:myorg/api.git
	branch = fix/PROJ-123
	tasktree-mode = work
[submodule "web"]
	path = web
	url = git@github.com:myorg/web.git
	branch = main
	tasktree-mode = follow
```

| Key | Meaning |
|---|---|
| `path` | Directory in the root. Equal to the name. |
| `url` | Clone URL as you gave it, or the URL an alias resolved to. |
| `branch` | The branch the repository tracks. Absent in `pin` mode. |
| `tasktree-mode` | `follow`, `work` or `pin`. |

The **gitlink** records what the repository was at when the root was last committed. The `branch` records intent. Together they replace V1's `Tasktree.yml` and its snapshot pin.

## `.tasktree/config.yml`

```yaml
apiVersion: tasktree.dev/v2
kind: TasktreeConfig
taskId: x3kjpk
bootstrap:
    - name: api-deps
      run: npm ci
      workdir: api
    - name: web-deps
      run: npm ci
      workdir: web
      env:
        CI: "1"
```

This file is committed and is the root marker: tasktree finds a checkout by walking up to the nearest file with `kind: TasktreeConfig`. `bootstrap` is optional. The file has a [JSON Schema](../reference/files.md#editor-schemas).

## Repository modes

The mode says how a repository moves when you run `sync`.

| Mode | Use | `sync` does |
|---|---|---|
| `follow` | Reference repository you read | Fast-forwards to `origin/<branch>` |
| `work` | Your own task branch | Fast-forwards from `origin/<your-branch>` only; reports how far the default branch has moved ("upstream drift"); never rebases |
| `pin` | A fixed tag or commit | Never moves |

In every mode a dirty repository, or a branch that cannot fast-forward, is skipped and reported. Change a mode with `tasktree mode <repo> <follow|work|pin>`. Switching to `follow` or `work` records the repository's current branch; switching to `pin` removes the recorded branch.

### How `add` chooses branch and mode

`add` clones from the local cache, so the first `add` of a URL fetches it and later ones are fast. Branch, starting point and mode follow from `--branch`, `--from` and `--mode`:

| Flags | Result | Mode |
|---|---|---|
| none | Default branch of the remote | `follow` |
| `--branch B`, `B` exists on the remote | Check out `B` | `work` |
| `--branch B`, `B` is new | Create `B` from the default branch | `work` |
| `--branch B --from F` | Create `B` from `F`. If `B` exists, `--from` is ignored and `add` says so. | `work` |
| `--from F`, `F` is a branch | Follow `F` | `follow` |
| `--from F`, `F` is a tag or commit | Detached at `F` | `pin` |

`--mode` overrides the derived mode. `follow` and `work` need a branch. `pin` never records one. `--name` sets the directory name; it defaults to the last part of the URL without `.git`.

When it adds to a checkout, `add` prints which case it used on stderr: `Using the default branch "main".`, `Using existing branch "B".`, `Creating new branch "B" from "main".` or `Checking out "F" without creating a branch.` The argument may be an alias. `add` registers `<name>` and `<owner>-<name>` as aliases for every URL you add. List and manage them with `tasktree alias list|add|rm`.

## Opening a tasktree

```bash
cd "$(tasktree open PROJ-123)"
```

`open` prints the path of the checkout and creates it first if this machine has none.

1. If a remote is configured and there is no checkout here, it fetches `main` and `task/<id>` first. This step is best effort and bounded to 10 seconds; on failure it continues with local state.
2. It clones the hub on `task/<id>` into `<tasksRoot>/<handle>`, or `--path`. If the task has no tasktree yet, it creates the first commit, with `.tasktree/config.yml` and a starter `AGENTS.md`.
3. For each repository it updates the local cache (if that fetch fails, it uses the cache as it is) and clones from the cache. It sets `origin` to the real URL.
4. It attaches each repository, because a freshly updated submodule is a detached HEAD:

   | Mode | State after `open` |
   |---|---|
   | `work`, `follow` | Local branch named by `.gitmodules` `branch`, at the recorded commit. Upstream is `origin/<branch>` if it exists. |
   | `pin` | Detached at the recorded commit |

5. It runs the bootstrap steps unless you pass `--skip-bootstrap`.

If the recorded commit of a repository is not in the cache or on the remote yet (for example it was never pushed from the other machine), the repository is left empty and uninitialized. `open` warns, and `info` shows state `uninitialized`. Run `tasktree sync` or `tasktree open` again when the commit exists. `open` never substitutes the tip of the branch.

If the task already has a checkout here, `open` only prints its path. A closed task cannot be opened.

## Committing the root

```console
$ tasktree commit -m "Record the redirect fix"
committed 08d1853 and pushed to the local hub
```

`commit` stages everything in the root, including the current commit of each repository, commits on `task/<id>` and pushes the branch to the hub. Without `-m` the message is `tasktree(<id>): checkpoint`. If nothing changed it prints `nothing to commit`.

- It does not commit inside repositories. Repositories with uncommitted changes produce a warning, because those changes are not recorded.
- It refuses to record a repository commit that is older than the one already recorded, which would silently revert a change from another machine. Pass `--allow-rewind` to override.
- It does not publish to a remote. That is `tasktree sync`.

Structural commands (`add`, `remove`, `mode`, `bootstrap add|rm`) commit their own change with a message such as `tasktree(<id>): add api`. They stage only the paths they changed, so your uncommitted root edits are not swept into the tool's commit. See [Files](../reference/files.md#commit-messages).

## Closing

A task that is set to `done` or `dropped` and has a checkout on this machine runs the **close flow**. It deletes the checkout and its hidden git directory, so it first makes sure that nothing would be lost.

```console
$ tasktree mark done
error: api has uncommitted changes
api has unpushed commits:
ced829a wip
[exit 6]
```

The steps:

1. **Subtask gate.** Refuse if the task has open subtasks. `--cascade` closes them first, deepest first. All gates for every checkout in the cascade are checked before anything changes.
2. **Loss gate.** Refuse if any of these hold. Each line is printed.
   - A repository has uncommitted changes, including untracked files.
   - A repository has commits, on `HEAD` or on any local branch, that are not reachable from `origin`.
   - A repository has a stash.
   - The root has ignored, untracked files (other than `.DS_Store` and `.tasktree/task.md`).
   - The root contains a git repository that is not registered as a repository of the tasktree.
   - A repository's `HEAD` is older than the commit recorded for it (a rewind).

   Tags are not counted: tags fetched from `origin` cannot be told apart from local ones. `--force` prints what will be lost and goes on.
3. **Commit the root** as `tasktree(<id>): close: <status>` and push the task branch to the hub.
4. **Record the status.**
5. **Push to the remote** (`main` and `task/<id>`), fast-forward only and best effort, if a remote is configured. A failure is a warning: the hub already has everything.
6. **Recheck the loss gate**, then delete the checkout directory, the hidden git directory, and the `local.yml` entry.

The branch `task/<id>` is kept. To work on the task again, run `tasktree mark active -t <handle>` and `tasktree open <handle>`. `--keep` records the status (and pushes it, best effort) but keeps the checkout and skips the loss gate. A task with open subtasks is still refused.

If your shell is inside the directory that was removed, tasktree prints `tasktree removed; run: cd ..`.

`tasktree remove <repo>` applies the same loss gate to one repository. It deletes the clone and commits the removal.

## Tasktrees without a checkout

A tasktree is a branch, so you can change it without a checkout. These commands accept `-t` and edit the branch directly when the task has no checkout on this machine:

```bash
tasktree add git@github.com:myorg/web.git -t PROJ-124
tasktree mode web pin -t PROJ-124
tasktree remove web -t PROJ-124
tasktree bootstrap add deps "npm ci" --workdir api -t PROJ-124
```

The base commits are resolved from the local cache, which is fetched, without a clone. The next `tasktree open PROJ-124` materializes the result. When the task has a checkout here, the same commands act on the checkout.

[Templates](templates.md) create a task and its tasktree this way.

## Bootstrap steps

Bootstrap steps are shell commands that prepare a fresh checkout, for example installing dependencies. They are stored in `.tasktree/config.yml` and run in order, with `sh -c`, from the step's `workdir` (relative to the root, default: the root). A step stops the sequence when it fails.

```console
$ tasktree bootstrap add deps "npm ci" --workdir api --env CI=1
added bootstrap step deps
$ tasktree bootstrap list
NAME  WORKDIR  RUN
deps  api      npm ci
$ tasktree bootstrap
==> [deps] npm ci
```

- `bootstrap run` (or plain `bootstrap`) runs the steps now.
- `open` runs them when it creates a checkout, unless `--skip-bootstrap` is set.
- Every step sees `TASKTREE_ROOT`, the absolute path of the tasktree root, plus its own `env` entries.
- Step names are unique. `bootstrap rm <name>` removes one.
- A step that fails exits with code 1.
