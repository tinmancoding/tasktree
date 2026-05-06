# Templates

Templates let you define reusable workspace configurations and instantiate them with a single command. Instead of running several `tasktree init`, `tasktree add`, and `tasktree annotate` commands every time you start a bugfix or feature, you define the pattern once and fill in variables at creation time.

## The problem templates solve

Without templates, starting a standard bugfix workspace looks like this:

```bash
tasktree init bugfix-BUG-123
cd bugfix-BUG-123
tasktree add git@github.com:myorg/api.git --branch fix/BUG-123
tasktree annotate set ticket BUG-123
tasktree annotate set type bugfix
```

With a `bugfix` template:

```bash
tasktree init --from bugfix ticket_number=BUG-123 repo=@api
```

One command. Same result.

## Template file format

Templates are YAML files with `kind: Template`. They declare parameters and a template body that mirrors the `Tasktree.yml` structure:

```yaml
apiVersion: tasktree.dev/v1
kind: Template

metadata:
  name: bugfix
  description: Single-repo bugfix workspace

parameters:
  - name: ticket_number
    required: true
    description: Ticket/issue identifier (e.g., BUG-123)
  - name: repo
    required: true
    description: Repository alias or URL
  - name: base_branch
    default: main
    description: Base branch to fork from

template:
  metadata:
    name: "bugfix-{{ticket_number}}"
    annotations:
      ticket: "{{ticket_number}}"
      type: bugfix

  spec:
    sources:
      - name: repo
        type: git
        git:
          url: "{{repo}}"
          ref: "{{base_branch}}"
          branch: "fix/{{ticket_number}}"
```

### Variable syntax

Variables are referenced with double braces anywhere in a string field:

```
{{variable_name}}                  # simple reference
{{variable_name | default:value}}  # with inline default
```

Variable names must match `[a-z][a-z0-9_]*` — lowercase letters, digits, and underscores, starting with a letter.

### Parameters section

Every `{{variable}}` reference in the template body must be declared in `parameters`:

| Field | Description |
|---|---|
| `name` | Variable identifier. Must match `[a-z][a-z0-9_]*`. |
| `required` | If `true`, the variable must be provided at init time (no default). |
| `default` | Value used when the variable is not supplied. |
| `description` | Human-readable explanation shown by `template show`. |

## Variable resolution order

When `tasktree init --from` runs, variables are resolved in this priority order — first source wins:

1. **CLI arguments** — `key=value` positional args after the template name
2. **Environment variables** — `TASKTREE_VAR_<UPPER_NAME>` (e.g. `TASKTREE_VAR_TICKET_NUMBER`)
3. **Parameter defaults** — `default:` field in `parameters`
4. **Inline defaults** — `{{var | default:value}}` in the template body

If a required variable remains unresolved after all sources are checked, the command exits with an error.

## Generated Tasktree.yml

The output of `init --from` is a fully resolved, standard `Tasktree.yml` with no variable syntax. All `{{references}}` are substituted before the file is written. The file is indistinguishable from one created manually — all existing commands (`apply`, `add`, `remove`, `annotate`, `status`) work on it without any knowledge of templates.

Template origin is tracked in annotations for reference:

```yaml
annotations:
  tasktree.dev/template: bugfix
  tasktree.dev/template-vars: "ticket_number=BUG-123,repo=@api,base_branch=main"
  ticket: BUG-123
  type: bugfix
```

## Template discovery

Tasktree discovers templates from three locations in priority order (first match wins for a given name):

| Priority | Location | Description |
|---|---|---|
| 1 | `./.tasktree/templates/*.yml` | Project-local templates in the current working directory |
| 2 | `~/.config/tasktree/templates/*.yml` | User-level templates shared across all projects |
| 3 | Built-in | Three templates embedded in the binary (`bugfix`, `feature`, `investigation`) |

A user-level template with the same `name` as a built-in takes precedence, letting you customize or replace the defaults.

## Built-in templates

Three templates ship with tasktree:

### `bugfix`

Single-repo bugfix workspace. Creates a `fix/<ticket>` branch from a configurable base.

```bash
tasktree init --from bugfix ticket_number=BUG-123 repo=@api
# → bugfix-BUG-123/Tasktree.yml
#   sources: repo @ fix/BUG-123 (from main)
```

Parameters: `ticket_number` (required), `repo` (required), `base_branch` (default: `main`)

### `feature`

Single-repo feature branch workspace.

```bash
tasktree init --from feature feature_name=payments repo=@api
# → feature-payments/Tasktree.yml
#   sources: repo @ feature/payments (from main)
```

Parameters: `feature_name` (required), `repo` (required), `base_branch` (default: `main`)

### `investigation`

Single-repo read-only investigation workspace. No branch is created — the repo is checked out at the specified ref in detached HEAD state.

```bash
tasktree init --from investigation ticket_number=INC-99 repo=@api ref=v2.3.1
# → investigate-INC-99/Tasktree.yml
#   sources: repo @ v2.3.1 (no branch)
```

Parameters: `ticket_number` (required), `repo` (required), `ref` (default: `main`)

## Writing your own templates

Create a file in `~/.config/tasktree/templates/` (or `.tasktree/templates/` for project-local templates):

```bash
mkdir -p ~/.config/tasktree/templates
cat > ~/.config/tasktree/templates/fullstack-feature.yml << 'EOF'
apiVersion: tasktree.dev/v1
kind: Template

metadata:
  name: fullstack-feature
  description: API + Web for a feature

parameters:
  - name: feature_name
    required: true
    description: Feature name (used in branch names)

template:
  metadata:
    name: "{{feature_name}}"
    annotations:
      type: feature

  spec:
    sources:
      - name: api
        type: git
        git:
          url: "@api"
          branch: "feature/{{feature_name}}"

      - name: web
        type: git
        git:
          url: "@web"
          branch: "feature/{{feature_name}}"
EOF
```

Validate it before using:

```bash
tasktree template validate fullstack-feature
# Template "fullstack-feature" is valid.
```

Then use it:

```bash
tasktree init --from fullstack-feature feature_name=payments
```

## Environment variable substitution

Variables can be sourced from the environment, which is useful for CI or shared scripts:

```bash
export TASKTREE_VAR_TICKET_NUMBER=BUG-123
export TASKTREE_VAR_REPO=git@github.com:myorg/api.git

tasktree init --from bugfix
```

The mapping is `TASKTREE_VAR_<UPPERCASE_NAME>` — underscores in the variable name map to underscores in the env var name.

## See also

- [`tasktree init --from`](../cli/init.md) — full flag reference
- [`tasktree template`](../cli/template.md) — list, show, and validate templates
- [Examples](../../examples/templates/) — example template files in the repository
