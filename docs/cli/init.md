# tasktree init

Initialize a new tasktree workspace.

## Synopsis

```
tasktree init [path] [flags]
tasktree init --from <template> [key=value...] [flags]
```

## Description

Creates a `Tasktree.yml` file in the target directory (defaulting to the current directory) and registers the workspace in the global registry at `~/.local/state/tasktree/registry.toml`.

If `path` does not exist, it is created.

When `--from` is provided, the workspace is created from a [template](../concepts/templates.md) instead. All `{{variable}}` references in the template are resolved to concrete values before writing `Tasktree.yml`. The result is a standard, fully resolved spec — no template syntax is ever stored in the file.

## Arguments

| Argument | Description |
|---|---|
| `path` | Directory to initialize. Defaults to `.` (current directory). Cannot be used with `--from`. |
| `key=value` | Variable bindings for template substitution. Only used with `--from`. |

## Flags

### Standard init

| Flag | Description |
|---|---|
| `--annotate key=value` | Set an annotation at init time. Repeatable. Key must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`. |

### Template init (`--from`)

| Flag | Description |
|---|---|
| `--from <template>` | Template name (e.g. `bugfix`) or file path (e.g. `./my-template.yml`) |
| `--name <name>` | Override the workspace directory name (otherwise derived from the template's `metadata.name`) |
| `--dir <path>` | Write the workspace into this directory (otherwise a new subdirectory is created) |
| `--apply` | Run `tasktree apply` immediately after creating the workspace |
| `--dry-run` | Print what would be created without writing any files |

## Examples

### Standard init

Initialize the current directory:

```bash
cd ~/work/feature-payments
tasktree init
# Initialized tasktree at /home/user/work/feature-payments
```

Initialize a new directory:

```bash
tasktree init ~/work/feature-payments
```

Initialize with annotations:

```bash
tasktree init ~/work/feature-payments \
  --annotate ticket=JIRA-4821 \
  --annotate owner=team-payments
```

### Template init

Create a bugfix workspace from the built-in `bugfix` template:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=@api
# Initialized tasktree at ./bugfix-BUG-123
```

Specify the target directory explicitly:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=@api \
  --dir ~/work/bug-123
```

Override an optional parameter that has a default:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=@api \
  base_branch=develop
```

Override the workspace name:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=@api \
  --name my-bugfix
```

Preview what would be created without writing anything:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=@api \
  --dry-run
# Dry run: would create tasktree at ./bugfix-BUG-123
#   name: bugfix-BUG-123
#   sources:
#     - repo (git): @api @ fix/BUG-123
```

Initialize and immediately materialize sources:

```bash
tasktree init --from bugfix \
  ticket_number=BUG-123 \
  repo=git@github.com:myorg/api.git \
  --apply
```

Initialize from a template file by path:

```bash
tasktree init --from ./my-templates/bugfix.yml \
  ticket_number=BUG-123 \
  repo=@api
```

## Variable resolution order

When `--from` is used, variables are resolved in this priority order:

1. `key=value` positional arguments on the command line
2. Environment variables (`TASKTREE_VAR_<UPPER_NAME>`, e.g. `TASKTREE_VAR_TICKET_NUMBER`)
3. Parameter `default` values declared in the template
4. Inline `{{var | default:value}}` defaults in the template body

## Errors

| Error | Cause |
|---|---|
| `tasktree metadata already exists at <path>` | A `Tasktree.yml` already exists at the target path. |
| `invalid annotation key "...": ...` | An `--annotate` key failed the format validation. |
| `template "..." not found in search paths` | The name passed to `--from` does not match any known template. |
| `missing required variable "..."` | A required template parameter was not supplied and has no default. |
| `template references unknown variable "..."` | The template body uses a `{{var}}` not declared in its `parameters`. |

## See also

- [`tasktree template list`](template.md) — discover available templates
- [`tasktree template show`](template.md#show) — inspect a template before using it
- [Templates concept guide](../concepts/templates.md) — template file format and authoring guide
