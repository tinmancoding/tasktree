# tasktree

tasktree manages tasks and the multi-repository workspaces for them in one tool. Git is the only place it keeps state.

A task is a Markdown record: a title, a status, references, a description and an append-only log. A workspace for a task is a plain directory with one full clone of each repository you work on. Both are versioned with ordinary git commits, so you can back them up, sync them to a second machine and read their history with tools you already have.

## Three concepts

| Concept | What it is |
|---|---|
| **Task** | A record on the `main` branch of a hidden tasks repository in `~/.tasktree`. It holds facts about the work: status, refs, notes. It never holds code. |
| **Tasktree** | The definition of a task's workspace: its repositories, files, symlinks and bootstrap steps. It lives on the branch `task/<id>`. A task has zero or one tasktree. |
| **Checkout** | A tasktree materialized in a directory on one machine, usually `<tasksRoot>/<handle>`. A task has at most one checkout per machine. |

See [The model](concepts/model.md) for the details.

## Example

```bash
tasktree setup --author alice --machine laptop
tasktree new "Fix login redirect" --jira PROJ-123
tasktree add git@github.com:myorg/api.git --branch fix/PROJ-123 -t PROJ-123
cd "$(tasktree open PROJ-123)"
# edit, commit and push inside api/ with plain git
tasktree commit -m "Record the fix"
tasktree note "Root cause: the return URL was decoded twice"
tasktree mark done
```

`open` creates `<tasksRoot>/PROJ-123` with `api/` cloned on `fix/PROJ-123`. `mark done` removes the directory after checking that no work would be lost. The task and its history stay.

## Where to go next

- New here: [Installation](getting-started/installation.md), then the [Quickstart](getting-started/quickstart.md).
- Coming from tasktree V1: [Importing V1 workspaces](getting-started/importing-v1.md). The V1 docs stay on the [`v1` branch](https://github.com/tinmancoding/tasktree/tree/v1/docs).
- How it works: [The model](concepts/model.md), [Task records](concepts/task-records.md), [Tasktrees](concepts/tasktrees.md), [Subtasks](concepts/subtasks.md), [Sync](concepts/sync.md), [Templates](concepts/templates.md).
- Using it with a coding agent: [Agents](guides/agents.md).
- Looking something up: [CLI reference](reference/cli/tasktree.md), [exit codes](reference/exit-codes.md), [JSON output](reference/json-output.md), [configuration](reference/configuration.md), [environment](reference/environment.md), [files](reference/files.md).
- Why it works this way: the [design document](design/tasktree-v2.md).
