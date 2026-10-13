# Configuration

Machine configuration lives in `~/.tasktree/config.yml` (or `$TASKTREE_HOME/config.yml`). It is per machine and is never synced. Read and change it with:

```console
$ tasktree config get tasksRoot
/home/alice/work/tasks
$ tasktree config set sync.jobs 8
```

`config get` prints the value, or an empty line when the key is unset. An unknown key exits with code 2. `tasktree setup` sets `tasksRoot`, `author`, `machine` and `remote` as well; it changes only the options you pass.

A file written by `setup --author alice --machine laptop --tasks-root ~/work/tasks` looks like this. Empty optional keys are left out.

```yaml
tasksRoot: /home/alice/work/tasks
author: alice
machine: laptop
sync:
    jobs: 4
```

## Keys

| Key | Default | Meaning |
|---|---|---|
| `remote` | none | URL of the tasks remote. `config set remote <url>` adds `origin` to the tasks repository and pushes `main` and every `task/*` branch. `config set remote ""` removes `origin`. See [Sync](../concepts/sync.md#setting-up-the-remote). |
| `tasksRoot` | `~/Developer/tasks` | Directory where checkouts are created, as `<tasksRoot>/<handle>`. Also where `doctor` looks for checkouts it lost track of, and where `import --move` puts workspaces. Use an absolute path: tasktree does not expand `~` itself, so rely on your shell to expand it. |
| `author` | none | Your name in log entries. Overridden by `TASKTREE_AUTHOR` and `--author`. Commands that write log entries (`note`, `new --template`) fail with `author is not configured` when no author is set. It cannot contain spaces or `@`. |
| `machine` | The host name up to the first dot | Machine name in log entries and in sync merge commits. It cannot contain spaces. Give each machine a different one. |
| `jira.baseUrl` | none | Base URL for Jira links. A `jira` reference `PROJ-123` links to `<baseUrl>/browse/PROJ-123` in `info`. Without it, Jira references have no link. tasktree never calls Jira. |
| `sync.jobs` | `4` | How many repositories `sync` fetches in parallel. A positive integer. |
| `sync.timeout` | none | An optional wall-clock limit, for example `90s` or `5m`, applied to each git transport command of an explicit transfer. For implicit best-effort calls the limit is the smaller of this value and 10 seconds. A positive Go duration. |

`sync.jobs` and `sync.timeout` are validated when you set them. The others are stored as given.

## Where each key is used

- `tasksRoot`: `open`, `new --open`, isolated subtasks, `import --move`, `doctor`.
- `author`, `machine`: every log entry; the fallback identity for commits made by tasktree when git has no `user.name` and `user.email`; sync merge commits.
- `remote`: `sync`, `open` (pre-fetch), `mark` (close push), `commit` (reminder text), `doctor`.
- `sync.jobs`, `sync.timeout`: `sync`, `open`, `mark`, `doctor`. See [Network use](../concepts/sync.md#network-use).

Other files in `~/.tasktree` that you may edit are [`aliases.yml`](files.md#aliasesyml) and the [templates](../concepts/templates.md). `local.yml` is state, not configuration.
