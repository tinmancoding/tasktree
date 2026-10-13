# JSON output

Every command that reads state accepts `--json`. The output is one JSON document on standard output, followed by a newline, with no other text. Warnings and progress still go to standard error. A failing command prints no JSON and exits with a [non-zero code](exit-codes.md).

| Command | Document |
|---|---|
| `list` | [array of tasks](#list) |
| `info` | [one task](#info) |
| `history` | [array of commits](#history) |
| `sync` | [sync report](#sync) |
| `resolve` | [resolve result](#resolve) |
| `doctor` | [findings and counts](#doctor) |
| `alias list` | [array of aliases](#alias-list) |
| `template list` | [array of templates](#template-list) |
| `template show` | [one template](#template-show) |
| `bootstrap list` | [array of steps](#bootstrap-list) |

Keys that are listed below always exist unless the table says they are optional. A list is never `null`: an empty list is `[]`. Only fields documented as "or null" (such as `parent`) can be `null`. Treat unknown keys as additions and ignore them. Object keys of `list` and `info` appear in alphabetical order; do not rely on key order anywhere. Timestamps are RFC 3339 in UTC. Task ids are 6 characters.

## Task fields

`list` and `info` share these fields for a task.

| Key | Type | Meaning |
|---|---|---|
| `id` | string | Task id |
| `handle` | string | Handle |
| `title` | string | Title |
| `status` | string | `incoming`, `next`, `active`, `blocked`, `done` or `dropped` |
| `parent` | string or null | Parent task **id** |
| `labels` | array of strings | Sorted labels |
| `refs` | array | References; see below |
| `createdAt`, `updatedAt` | string | Timestamps |
| `description` | string | Description text |
| `log` | array | Log entries, oldest first |
| `conflict` | array of strings | Optional. Present only when fields are in conflict. |
| `progress` | object | Optional. Present only when the task has subtasks: `{"closed": n, "total": n}`, counting direct subtasks. |

A log entry is an object with `id`, `timestamp`, `kind` (`note`, `comment`, `decision` or `ref`), `author`, `machine` and `body`.

In `list`, a reference is `{"type": "jira|pr|url", "value": "..."}`. In `info`, it also has `url`, when tasktree can build a link: always for `pr` and `url`, and for `jira` only when `jira.baseUrl` is configured.

## list

```console
$ tasktree list --json
[{"createdAt":"2026-10-13T14:40:22Z","description":"","handle":"PROJ-200","id":"b4wr34","labels":["payments"],"log":[...],"parent":null,"progress":{"closed":0,"total":1},"refs":[{"type":"jira","value":"PROJ-200"}],"status":"incoming","tasktree":"here","title":"Payments retry","updatedAt":"2026-10-13T14:40:22Z"}]
```

An array of [task objects](#task-fields), oldest first, filtered like the table output (`--all`, `--status`, `--label`, `--ref`). An empty result is `[]`. Each object has one more key:

| Key | Type | Meaning |
|---|---|---|
| `tasktree` | string | `here` (a checkout exists on this machine), `defined` (a tasktree branch but no checkout here) or `none` |

## info

```console
$ tasktree info --json
{
  "createdAt": "2026-10-13T14:40:22Z",
  "description": "",
  "focus": null,
  "handle": "PROJ-200",
  "id": "b4wr34",
  "labels": ["payments"],
  "log": [
    {
      "author": "alice",
      "body": "Retry budget is 3",
      "id": "g3g3y4",
      "kind": "note",
      "machine": "laptop",
      "timestamp": "2026-10-13T14:40:22Z"
    }
  ],
  "parent": null,
  "progress": {"closed": 0, "total": 1},
  "refs": [
    {"type": "jira", "url": "https://example.atlassian.net/browse/PROJ-200", "value": "PROJ-200"},
    {"type": "pr", "url": "https://github.com/myorg/api/pull/42", "value": "myorg/api#42"}
  ],
  "status": "incoming",
  "subtasks": [{"handle": "add-retry", "id": "ypbatb", "status": "incoming"}],
  "tasktree": {
    "path": "/home/alice/work/tasks/PROJ-200",
    "repos": [
      {
        "name": "api",
        "path": "api",
        "mode": "work",
        "branch": "feature/retry",
        "head": "3780b194e35c",
        "dirty": false,
        "ahead": 0,
        "behind": 0,
        "unpushed": 0,
        "uninitialized": false
      }
    ],
    "rootDirty": 0,
    "state": "here"
  },
  "title": "Payments retry",
  "updatedAt": "2026-10-13T14:40:22Z"
}
```

(The example is reformatted. The tool prints one line.)

The [task fields](#task-fields) plus:

| Key | Type | Meaning |
|---|---|---|
| `subtasks` | array | Direct subtasks: `{"id", "handle", "status"}`. Empty array when none. |
| `focus` | string or null | Present only when the command ran inside a checkout. The handle of the focused subtask, or `null`. |
| `tasktree` | object | The tasktree, in one of three shapes below |

`tasktree` has a `state`:

- `"here"`: the task has a checkout on this machine.

  | Key | Type | Meaning |
  |---|---|---|
  | `path` | string | Checkout root |
  | `rootDirty` | number | Count of uncommitted paths in the root, not counting changes inside repositories. A repository at a new commit counts as one. |
  | `repos` | array | One object per repository |

  Each repository: `name`, `path`, `mode` (`follow`, `work` or `pin`), `branch` (`(detached)` when detached), `head` (12-character commit), `dirty` (boolean, uncommitted changes), `ahead` and `behind` (commits relative to `origin/<branch>`), `unpushed` (commits on any local branch that are not on any `origin` branch) and `uninitialized` (boolean). An uninitialized repository has empty `head` and zero counts.

- `"defined"`: a tasktree branch exists but there is no checkout here. `repos` entries have `name`, `path`, `mode`, `branch` and `gitlink` (the full recorded commit).

- `"none"`: no tasktree. `repos` is `[]`.

## history

```console
$ tasktree history --json
[{"timestamp":"2026-10-13T14:29:03Z","author":"alice","subject":"task(c9sezd): status incoming → done"}, ...]
```

An array of `{"timestamp", "author", "subject"}`, newest first. `author` is the git author of the commit.

## sync

```console
$ tasktree sync --json
{"steps":["fetched hub origin","reconciled main and task refs","pushed leased hub refs"],"dirty":[],"diverged":[],"skipped":[],"unpushedGitlinks":[],"checkoutsBehind":[],"closedTaskCheckouts":[],"failed":[]}
```

| Key | Type | Meaning |
|---|---|---|
| `steps` | array of strings | What happened, in order |
| `dirty` | array | Repositories with uncommitted changes that were left alone. Also the root path when it was dirty. |
| `diverged` | array | Task refs whose local and remote histories both moved |
| `skipped` | array | Repositories that did not move, with the reason |
| `unpushedGitlinks` | array | Recorded commits not reachable from `origin` |
| `checkoutsBehind` | array | Paths of checkouts behind the hub |
| `closedTaskCheckouts` | array | Paths of checkouts of closed tasks |
| `failed` | array | Failed steps. If not empty, the exit code is 1. |

All values are strings, and every array is present, possibly empty. The document is printed also when the sync fails, so read the exit code as well. The exit code is not part of the document.

## resolve

```console
$ tasktree resolve --json
{"localCommits":[],"remoteCommits":[],"paths":[],"autoMerged":[],"nothingToResolve":true,"report":{"steps":[],"dirty":[],"diverged":[],"skipped":[],"unpushedGitlinks":[],"checkoutsBehind":[],"closedTaskCheckouts":[],"failed":[]}}
```

| Key | Type | Meaning |
|---|---|---|
| `localCommits`, `remoteCommits` | array of strings | `<short hash> <subject>` lines that exist on one side only |
| `paths` | array of strings | Paths changed differently on both sides |
| `autoMerged` | array of strings | Paths changed on one side only |
| `commit` | string | Optional. The merge commit, present after a successful resolve. |
| `nothingToResolve` | boolean | `true` when the histories have not diverged |
| `report` | object | A [sync report](#sync) for the steps run after the merge |

## doctor

```console
$ tasktree doctor --json
{"findings":[{"code":"checkout-missing","severity":"error","subject":"8qtbp7","message":"recorded checkout path does not exist; hidden clone has no committed or stashed work at risk; delete it with doctor --fix --prune-clones only if the checkout folder is gone for good","fixable":true,"fixed":false,"destructive":true},{"code":"no-remote","severity":"warning","subject":"/home/alice/.tasktree","message":"no remote configured; ~/.tasktree is the only copy; make a backup","fixable":false,"fixed":false}],"errors":1,"warnings":1}
```

| Key | Type | Meaning |
|---|---|---|
| `findings` | array | Findings, errors first. `[]` when there are none. |
| `errors`, `warnings` | number | Counts of findings that are not fixed |
| `findings[].destructive` | boolean | Present and `true` when the repair deletes a hidden clone; applied only with `--fix --prune-clones` |

A finding has `code`, `severity` (`error` or `warning`), `subject`, `message`, `fixable` and `fixed` (true after `--fix` repaired it). The codes are listed in [Doctor and recovery](../guides/doctor-and-recovery.md#codes). The exit code is 1 when `errors` is greater than zero.

## alias list

```console
$ tasktree alias list --json
[{"alias":"api","url":"git@github.com:myorg/api.git"},{"alias":"myorg-api","url":"git@github.com:myorg/api.git"}]
```

An array of `{"alias", "url"}` sorted by alias, `[]` when there are none.

## template list

```console
$ tasktree template list --json
[{"name":"bugfix","description":"Single-repository bug fix tasktree","source":"builtin"}, ...]
```

An array of `{"name", "description", "source"}`. `source` is `builtin` or `user`.

## template show

```console
$ tasktree template show bugfix --json
{"name":"bugfix","description":"Single-repository bug fix tasktree","source":"builtin","parameters":[{"name":"ticket_number","required":true,"default":"","description":"Ticket or issue identifier"}, ...],"raw":"apiVersion: tasktree.dev/v2\n..."}
```

| Key | Type | Meaning |
|---|---|---|
| `name`, `description` | string | From `metadata` |
| `source` | string | `builtin`, `user` or `file` |
| `parameters` | array | `{"name", "required", "default", "description"}`. `default` is `""` when the parameter has none. |
| `raw` | string | The template YAML exactly as stored |

## bootstrap list

```console
$ tasktree bootstrap list --json
[{"name":"deps","run":"npm ci","workdir":"api","env":{"CI":"1"}}]
```

An array of steps in run order. `workdir` is `""` when unset and `env` is `{}` when none. `[]` when there are no steps.
