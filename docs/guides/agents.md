# Working with coding agents

A tasktree is built to be shared between you and coding agents. The task record is the shared memory, and a few rules keep everybody out of each other's way.

## What an agent finds in a tasktree

| File | Purpose |
|---|---|
| `AGENTS.md` | Rules for agents. Written once when the tasktree is created. |
| `.tasktree/task.md` | A generated, read-only copy of the task record. Kept current by `open`, `sync` and every record write. |
| `<repo>/` | Normal git repositories. Agents use git here. |

Many agent tools read `AGENTS.md` from the working directory automatically.

## The starter `AGENTS.md`

When tasktree creates a tasktree, whether through `open`, `add -t`, a template or `import`, it writes this file once. After that it belongs to you. tasktree never rewrites it, so edit it freely or give your template its own `AGENTS.md`.

```markdown
# Fix login redirect (PROJ-123)

This folder is a tasktree for task `PROJ-123` (id `mqcf4c`), managed by `tasktree`.

## Rules for agents

- The task record is the source of truth for status, notes, decisions, and references. Read it with `tasktree info` or `.tasktree/task.md` (a generated copy — never edit it).
- Record progress with `tasktree note "..."`; use `--kind decision`, `--kind comment`, or `--kind ref` when it fits. Set `TASKTREE_AUTHOR` to your agent name.
- Never run git in this folder's root; it is intentionally not a git repository. Record root files and repository commits with `tasktree commit -m "..."`.
- Inside each repository folder, git works normally: branch, commit, push.
- Add or remove repositories with `tasktree add <alias|url>` and `tasktree remove <name>`. Never `git clone` into this folder.
- Change task status (`tasktree mark ...`) only when asked.

## Notes
```

An isolated subtask gets the same file with one extra line after the first paragraph: ``Parent task: `PROJ-200` — read it with `tasktree info -t PROJ-200`.``

## Telling the agent who it is

Every log entry carries an author. Set it per agent, so the log shows who wrote what:

```bash
export TASKTREE_AUTHOR=claude
tasktree note "Found the cause in the URL helper"
```

The author is `--author`, then `TASKTREE_AUTHOR`, then the `author` key of the machine config. It cannot contain spaces or `@`. The entry also records the machine name from the config.

## Reading the task: `--json`

Commands that read state can print JSON. Use it instead of parsing tables. The exact shapes are in [JSON output](../reference/json-output.md).

```bash
tasktree info --json
tasktree list --json
tasktree history --json
tasktree sync --json
tasktree doctor --json
```

`info --json` contains the whole record, including the log, the subtasks, and the repository table of the checkout.

Do not parse the human-readable output of other commands, and read errors from standard error. Normal results go to standard output. Warnings and progress go to standard error.

## Exit codes

Agents can branch on the exit code instead of the message:

| Code | Meaning | A sensible reaction |
|---|---|---|
| 0 | Success | Continue |
| 1 | Other error | Read the message |
| 2 | Wrong usage or invalid value | Fix the command |
| 3 | Task, repository or template not found | Check the name |
| 4 | Not in a tasktree, or the task has no checkout | `cd` into one, or pass `-t`, or `open` it |
| 5 | Conflict | Stop and tell the user; for example run `commit` before `sync`, or `resolve` |
| 6 | A safety gate refused | Stop and report; never add `--force` on your own |

The full list is in [Exit codes](../reference/exit-codes.md).

## Rules that keep agents safe

1. **Never run git in the tasktree root.** It is not a repository. Use `tasktree commit` for the root and plain git inside each repository.
2. **Do not pass `--force`**, `--allow-rewind` or `--cascade` unless the user asked for it. Those flags exist to override a safety gate.
3. **Do not run `tasktree mark`** unless asked. Closing a task deletes its checkout.
4. **Do not edit `.tasktree/task.md`.** Use `note`, `ref add`, `label add` and `info edit`.
5. **Do not open an editor.** `info edit` without `--file` or `--stdin` starts `$VISUAL` or `$EDITOR` and waits. Pass the new description on standard input:

   ```bash
   printf 'The login page loops when the return URL is encoded.\n' | tasktree info edit --stdin
   ```

6. **Push code with `tasktree push`**, or with git inside the repository on a branch that is not `main`, `master`, `release/*` or the remote's default branch. `tasktree sync` publishes records and the root, never repository code.
7. **Record decisions as they happen.** A `--kind decision` note costs one command and saves the next session from rediscovering it.

## Several agents at once

Two agents in one directory must not share a focus. Give each its own target with the environment:

```bash
TASKTREE_TASK=add-retry-to-client TASKTREE_AUTHOR=agent-a tasktree note "client side done"
TASKTREE_TASK=add-backoff TASKTREE_AUTHOR=agent-b tasktree note "server side done"
```

`TASKTREE_TASK` takes a handle or an id and applies to record commands only (`info`, `note`, `mark`, `ref`, `label`, `history`). Writes are serialized by a lock on the tasks repository, so concurrent notes queue for a few milliseconds and do not clash.

If agents need their own branches, create [isolated subtasks](../concepts/subtasks.md#isolated-subtasks). Each gets its own directory and branches, forked from the parent's current commit.

## A snippet for your global agent instructions

Add this to the instructions your agent loads everywhere, for example `~/.config/AGENTS.md` or its equivalent:

```markdown
## Tasks

I track work with `tasktree`. If the working directory is inside a tasktree (it has `.tasktree/config.yml`
above it), read `AGENTS.md` there first. Use `tasktree info` to read the task and `tasktree note` to record
progress and decisions. Never run git in a tasktree root. Never run `tasktree mark`, `--force` or `--cascade`
unless I ask.
```
