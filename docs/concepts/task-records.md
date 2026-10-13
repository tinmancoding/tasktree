# Task records

A task record is one Markdown file, `tasks/<id>.md`, on the `main` branch of the tasks repository. It is the personal equivalent of a ticket. `tasktree info` reads it, and the commands in this page write it.

## Format

```markdown
---
id: mqcf4c
handle: PROJ-123
title: Fix login redirect
status: active
parent: null
labels: [auth, bug]
refs:
  - jira: PROJ-123
  - pr: myorg/api#42
  - url: https://example.com/wiki/login
createdAt: 2026-10-13T14:24:45Z
updatedAt: 2026-10-13T14:24:46Z
---

## Description

The login page loops when the return URL is encoded.


## Log

### 2026-10-13T14:24:46Z note alice@laptop {#e-3d1pk1}
Retry budget is 3
```

A record has three parts with different write rules:

| Part | Write rule | Commands |
|---|---|---|
| Frontmatter | Typed fields. Merged field by field when machines diverge. | `new`, `mark`, `ref`, `label`, `info set`, `info ack` |
| `## Description` | Free text. Replaced as a whole. | `info edit` |
| `## Log` | Append-only. Entries are never edited. | `note` |

### Frontmatter

| Key | Meaning |
|---|---|
| `id` | Immutable, 6 characters from `0123456789abcdefghjkmnpqrstvwxyz`. Names the branch `task/<id>` and the clone directory. Random, so two offline machines do not collide. |
| `handle` | Mutable, human-facing. Used in directory names and on the command line. |
| `title` | One line. |
| `status` | One of the six statuses below. |
| `parent` | The `id` of the parent task, or `null`. See [Subtasks](subtasks.md). |
| `labels` | Sorted list of labels. |
| `refs` | Typed references to external items. |
| `createdAt`, `updatedAt` | UTC timestamps. |
| `conflict` | Only present after a sync conflict. Lists the fields in conflict. |

Unknown frontmatter keys, and hand edits you make in the hub, are preserved when tasktree rewrites a record.

### Syntax rules

- `## Description` and `## Log` are reserved headings. `## Log` is always the last section. A description or a note that contains either heading, or a line that looks like a log entry header, is rejected.
- A log entry is a header line followed by its body, up to the next header:

  ```text
  ### <RFC 3339 UTC time> <kind> <author>@<machine> {#e-<entry id>}
  ```

- The kind is `note`, `comment`, `decision` or `ref`. The entry id uses the same 6-character alphabet as task ids and is unique within the file.
- The author comes from `--author`, then `TASKTREE_AUTHOR`, then the `author` config key. Neither author nor machine may contain spaces, and the author may not contain `@`.

## Handles and ids

`tasktree new` picks the handle in this order: `--handle`, then `--jira`, then a slug of the title (lower case, non-alphanumeric runs replaced by `-`, at most 40 characters). A handle matches `[A-Za-z0-9._-]` and has at most 64 characters.

Handles are unique on one machine: `new` and `info set handle` refuse a duplicate with exit code 5 (`new` also suggests a free handle). After a [sync](sync.md) two machines can still end up with the same handle. A command that is given such a handle fails with exit code 5 and lists the ids, and `doctor` reports `duplicate-handle`. Use the id, or fix one with `tasktree info set handle <new> -t <id>`.

Every command that takes a task accepts either form.

## Statuses

| Category | Statuses |
|---|---|
| Open | `incoming`, `next`, `active`, `blocked` |
| Closed | `done`, `dropped` |

- A new task is `incoming` (change with `new --status`).
- `tasktree mark <status>` is the only command that changes the status. `info set` refuses it.
- `list` shows open tasks. `--all` adds closed ones, and `--status done` shows just those.
- Setting a closed status on a task with a checkout runs the close flow, which deletes the checkout. See [Closing a tasktree](tasktrees.md#closing).
- A closed task cannot be opened. Reopen it first with `tasktree mark active -t <handle>`.

## References

References link a task to things outside tasktree. tasktree stores them and builds links. It never calls the external system.

```bash
tasktree ref add jira PROJ-123 -t PROJ-123
tasktree ref add pr myorg/api#42 -t PROJ-123
tasktree ref add url https://example.com/wiki/login -t PROJ-123
tasktree ref rm pr myorg/api#42 -t PROJ-123
```

| Type | Value | Link shown by `info` |
|---|---|---|
| `jira` | A key such as `PROJ-123` | `<jira.baseUrl>/browse/PROJ-123`, only when `jira.baseUrl` is set |
| `pr` | `owner/repo#N` | `https://github.com/owner/repo/pull/N` |
| `url` | Any URL | The URL |

Adding a reference that exists, or removing one that does not, changes nothing. Removing a missing reference exits with code 3. `--jira` on `new` adds the first `jira` reference.

Find tasks by reference with an exact match:

```bash
tasktree list --ref jira:PROJ-123
tasktree list --ref pr:myorg/api#42 --all
```

## Labels

```bash
tasktree label add bug auth -t PROJ-123
tasktree label rm auth -t PROJ-123
tasktree list --label bug --label auth
```

A label matches `[a-z0-9][a-z0-9._-]*`. `list --label` may repeat; a task must have every label given.

## Conflict flags

When two machines change the same field and `sync` merges them, the merge keeps your local value, writes the remote value into a `comment` log entry, and adds the field to `conflict:`. The record then looks like this:

```markdown
---
id: tm8c3x
handle: PROJ-123
title: Fix redirect on login
status: incoming
parent: null
labels: []
refs:
  - jira: PROJ-123
createdAt: 2026-10-13T14:33:24Z
updatedAt: 2026-10-13T14:33:27Z
conflict:
  - title
---

## Description


## Log
### 2026-10-13T14:33:27Z comment alice@desktop {#e-tbjnqy}
Sync conflict on title: remote value (commit 5118de5 by Alice at 2026-10-13T16:33:27+02:00) was: Fix login redirect loop
```

`list` shows a `!` after the status of a flagged task, `info` prints a `Conflict:` line, and `doctor` reports `conflict-flag`. Clear a flag in one of three ways:

- `tasktree info ack title` clears it and keeps the local value.
- `tasktree info set title "..."` sets a value and clears the flag.
- `tasktree info edit` clears a `description` flag.

`info ack` exits 3 if the field is not flagged. See [Sync](sync.md#conflicts).

## History

The changelog of a task is the git history of its file. tasktree writes one commit per change, with a fixed message:

| Change | Commit message |
|---|---|
| `new` | `task(<id>): create "<title>"` |
| `new --template` | `task(<id>): create from template <name>` |
| `mark` | `task(<id>): status <old> → <new>` |
| `note` | `task(<id>): <kind> added` |
| `info edit` | `task(<id>): description updated` |
| `info set` | `task(<id>): title updated`, `handle updated`, `parent set to <id>` or `parent cleared` |
| `info ack` | `task(<id>): conflict acknowledged: <field>` |
| `ref add`, `ref rm` | `task(<id>): ref added`, `ref removed` |
| `label add`, `label rm` | `task(<id>): labels added`, `labels removed` |

```console
$ tasktree history -t PROJ-123
2026-10-13T14:29:03Z  alice  task(c9sezd): status incoming → done
2026-10-13T14:29:03Z  alice  task(c9sezd): decision added
2026-10-13T14:29:03Z  alice  task(c9sezd): note added
2026-10-13T14:29:01Z  alice  task(c9sezd): create "Fix login redirect"
```

The author column is the git author of the commit. It is your git `user.name` when git has one configured, otherwise the `author` config key.

## The generated copy

Inside a checkout, `.tasktree/task.md` is a copy of the record. tasktree regenerates it on `open`, `sync` and after every record write. It is ignored by git. Read it, never edit it: edits are overwritten. Use `tasktree info` and the commands above.
