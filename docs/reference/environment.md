# Environment variables

## Variables tasktree reads

| Variable | Used by | Meaning |
|---|---|---|
| `TASKTREE_HOME` | All commands | The directory that holds all tasktree data. Default `~/.tasktree`. A relative value is made absolute. Use it to keep a second, independent set of tasks. |
| `TASKTREE_TASK` | Record commands | Handle or id of the task that `info`, `note`, `mark`, `ref`, `label`, `history` and `info set|edit|ack` act on when `-t` is not given. It beats the focused subtask and the tasktree's own task. It must name an existing task, or the command fails. Structural commands ignore it. See [Subtasks](../concepts/subtasks.md#targeting). |
| `TASKTREE_AUTHOR` | Commands that write log entries | The author of log entries. Overridden by `--author`, and used before the `author` config key. |
| `TASKTREE_CACHE_DIR` | `add`, `open`, `sync`, `new --template` | Directory of the bare clone cache. Default: the user cache directory plus `tasktree/repos`, that is `~/Library/Caches/tasktree/repos` on macOS and `$XDG_CACHE_HOME/tasktree/repos` (default `~/.cache/tasktree/repos`) on Linux. The cache can be deleted at any time; it is rebuilt on demand. |
| `TASKTREE_VAR_<NAME>` | `new --template` | Value for the template parameter `<name>`, with the name upper-cased: `TASKTREE_VAR_REPO` for `repo`. Beaten by `name=value` arguments, beats parameter defaults. See [Templates](../concepts/templates.md#variables). |
| `TASKTREE_V1_REGISTRY` | `import --all` | Path of the V1 registry. Default `~/.local/state/tasktree/registry.toml`. |
| `TASKTREE_V1_ALIASES_FILE` | `setup` | Path of the V1 alias file that `setup` copies to `aliases.yml` when that file does not exist. Default `~/.config/tasktree/repos.yml`. |
| `VISUAL`, `EDITOR` | `info edit` | The editor, tried in that order, then `vi`. Only used when neither `--file` nor `--stdin` is given. |
| `HOME` | All commands | Base for the defaults above. |

## Variables tasktree sets

| Variable | Where | Meaning |
|---|---|---|
| `TASKTREE_ROOT` | Bootstrap steps | The absolute path of the tasktree root. A step's own `env` may override it. |
| `GIT_TERMINAL_PROMPT=0` | Git commands that fetch, push or clone | Git never prompts; a command that needs a password fails. |
| `GIT_SSH_COMMAND` | The same git commands, when neither it nor `core.sshCommand` is set | `ssh -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4` |

## Git variables

tasktree removes the repository-context variables (the list printed by `git rev-parse --local-env-vars`, such as `GIT_DIR`, `GIT_WORK_TREE` and `GIT_INDEX_FILE`) from the environment of every git process and bootstrap step it starts. A stray `GIT_DIR` in your shell therefore cannot redirect tasktree to the wrong repository. Authentication and transport settings (`GIT_SSH_COMMAND`, `GIT_ASKPASS`, `GIT_CONFIG_*`, proxy variables) pass through. tasktree itself never exports `GIT_*` variables.

## Test hooks

The test suite sets some `TASKTREE_TEST_*` variables to inject failures. They are not a supported interface and are not documented. Do not set them.
