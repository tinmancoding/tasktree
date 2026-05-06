# tasktree template

Manage workspace templates.

## Synopsis

```
tasktree template <subcommand> [flags]
```

## Subcommands

| Subcommand | Description |
|---|---|
| [`list`](#list) | List all available templates |
| [`show <name\|path>`](#show) | Display full details for a template |
| [`validate <name\|path>`](#validate) | Check a template file for errors |

---

## list

List all available templates from all discovery paths.

```
tasktree template list
```

Output columns:

| Column | Description |
|---|---|
| `NAME` | Template name used with `--from` |
| `DESCRIPTION` | Short description from `metadata.description` |
| `PARAMETERS` | Required parameters the template expects |

**Example:**

```
$ tasktree template list
NAME           DESCRIPTION                                      PARAMETERS
bugfix         Single-repo bugfix workspace                     ticket_number, repo
feature        Single-repo feature branch workspace             feature_name, repo
investigation  Single-repo investigation workspace (read-only)  ticket_number, repo
```

Three built-in templates ship with tasktree and are always available. Templates in `~/.config/tasktree/templates/` or `.tasktree/templates/` are listed first and shadow built-ins of the same name.

---

## show

Display full details for a named template, including all parameters and a preview of the generated workspace structure.

```
tasktree template show <name|path>
```

| Argument | Description |
|---|---|
| `name\|path` | Template name to look up, or a file path ending in `.yml` / `.yaml` |

**Example:**

```
$ tasktree template show bugfix

Name:        bugfix
Description: Single-repo bugfix workspace

Parameters:
  ticket_number  (required)  Ticket/issue identifier (e.g., BUG-123)
  repo           (required)  Repository alias or URL
  base_branch    (optional)  Base branch to fork from [default: main]

Template Preview:
  metadata.name: bugfix-{{ticket_number}}
  sources:
    - repo (git): {{repo}}, branch: fix/{{ticket_number}}
```

---

## validate

Validate a template file for structural and schema errors.

```
tasktree template validate <name|path>
```

| Argument | Description |
|---|---|
| `name\|path` | Template name to look up, or a file path |

Validation checks:
- `kind` must be `Template`
- `metadata.name` must be non-empty
- All parameter names must match `[a-z][a-z0-9_]*`
- Every `{{variable}}` reference in the template body must be declared in `parameters`
- Typo detection: unknown variable names are checked against declared parameters and a suggestion is shown if a close match is found

**Example:**

```
$ tasktree template validate bugfix
Template "bugfix" is valid.

$ tasktree template validate ./my-template.yml
Error: template references unknown variable "tocket_number" (did you mean "ticket_number"?)
```

---

## Errors

| Error | Cause |
|---|---|
| `template "..." not found in search paths` | No template with that name exists in any discovery path |
| `template references unknown variable "..."` | Template body uses a `{{var}}` that is not declared in `parameters` |
| `invalid variable name "..."` | A parameter name or variable reference does not match `[a-z][a-z0-9_]*` |

## See also

- [`tasktree init --from`](init.md) — create a workspace from a template
- [Templates concept guide](../concepts/templates.md) — template file format, variable syntax, discovery paths
