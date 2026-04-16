# tasktree annotate

Manage annotations for the current tasktree.

## Synopsis

```
tasktree annotate set <key> <value>
tasktree annotate unset <key>
tasktree annotate list
```

## Description

Annotations are arbitrary key/value string pairs stored in the `metadata.annotations` map of `Tasktree.yml`. They are designed for human-readable context: purpose, ownership, linked tickets, documentation URLs, sprint info, and so on.

Unlike `labels` (which are short, machine-readable identifiers for filtering), annotations are free-form prose intended to be read and displayed by developers.

Annotations are shown in the output of [`tasktree status`](status.md) when any are set.

## Subcommands

### annotate set

```
tasktree annotate set <key> <value>
```

Set or update the annotation `<key>` to `<value>`. If the key already exists it is overwritten. The key must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` and must not exceed 128 characters.

### annotate unset

```
tasktree annotate unset <key>
```

Remove the annotation with the given key. This is a no-op if the key does not exist, making it safe for scripting.

### annotate list

```
tasktree annotate list
```

Print all annotations for the current tasktree as a two-column table sorted by key. Prints `"No annotations set."` when there are none.

## Key format

| Constraint | Detail |
|---|---|
| Non-empty | Key must not be empty |
| Maximum length | 128 characters |
| Pattern | `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` |

Dots allow simple namespacing, e.g. `jira.ticket` or `github.pr`.

## Examples

Set annotations:

```bash
tasktree annotate set purpose "Integration testing for Q3 payments feature"
tasktree annotate set owner team-payments
tasktree annotate set ticket JIRA-4821
tasktree annotate set sprint Sprint-42
tasktree annotate set docs https://wiki.example.com/payments-workspace
```

List all annotations:

```bash
tasktree annotate list
# KEY      VALUE
# docs     https://wiki.example.com/payments-workspace
# owner    team-payments
# purpose  Integration testing for Q3 payments feature
# sprint   Sprint-42
# ticket   JIRA-4821
```

Remove an annotation:

```bash
tasktree annotate unset sprint
```

Use namespaced keys for structured metadata:

```bash
tasktree annotate set jira.ticket JIRA-4821
tasktree annotate set jira.sprint Sprint-42
tasktree annotate set github.pr 1234
```

## Errors

| Error | Cause |
|---|---|
| `invalid annotation key "...": key must match ...` | Key failed the format validation. |
| `Not inside a tasktree` | No `Tasktree.yml` found walking up from the current directory. |

## See also

- [Workspace Spec — Annotations](../concepts/workspace-spec.md#annotations)
- [`tasktree init --annotate`](init.md) — set annotations at workspace creation time
- [`tasktree status`](status.md) — displays annotations in its output header
