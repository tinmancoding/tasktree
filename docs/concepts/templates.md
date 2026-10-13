# Templates

A template creates a task and, optionally, its tasktree in one command. It fills in a record (title, handle, labels, description) and a tasktree definition (repositories, files, symlinks, bootstrap steps) from a few parameters.

```bash
tasktree new --template bugfix ticket_number=PROJ-9 repo=api
```

```console
$ tasktree new --template bugfix ticket_number=PROJ-9 repo=api
PROJ-9 hhrzrw
$ tasktree info -t PROJ-9
ID: hhrzrw
Handle: PROJ-9
Title: Fix PROJ-9
Status: incoming
...
Tasktree: defined (not checked out here)
NAME  MODE  BRANCH      GITLINK
api   work  fix/PROJ-9  25cc2b9216e0ba8a983ece1283ebb35a4cff392e
```

The template creates the definition only. Run `tasktree open PROJ-9` to check it out, or add `--open` to `new` to do both.

## Where templates come from

- **Built-in:** `bugfix`, `feature` and `investigation`. Each takes one repository.
- **Yours:** every `*.yml` and `*.yaml` file in `~/.tasktree/templates/`. A template is found by the `metadata.name` inside the file, not by the file name. A template of yours replaces a built-in one with the same name.
- **A file:** `--template ./path/to/file.yml`. An argument that contains a `/` or ends in `.yml` or `.yaml` is read as a file.

```console
$ tasktree template list
NAME           SOURCE   DESCRIPTION
bugfix         builtin  Single-repository bug fix tasktree
feature        builtin  Single-repository feature tasktree
investigation  builtin  Single-repository investigation tasktree
$ tasktree template show bugfix
PARAMETER      REQUIRED  DEFAULT  DESCRIPTION
ticket_number  true               Ticket or issue identifier
repo           true               Repository alias or URL
base_branch    false     main     Base branch
---
apiVersion: tasktree.dev/v2
kind: Template
...
$ tasktree template validate examples/templates/fullstack-feature.yml
template fullstack-feature is valid
```

`template list --json` and `template show --json` are described in [JSON output](../reference/json-output.md).

## Format

This is `examples/templates/fullstack-feature.yml` from the repository. It uses every section.

```yaml
apiVersion: tasktree.dev/v2
kind: Template
metadata:
  name: fullstack-feature
  description: API and web repositories for one feature
parameters:
  - {name: feature_name, required: true, description: "Feature name; used in the handle, title and branch names"}
  - {name: base_branch, default: main, description: Branch to start both feature branches from}
task:
  title: "Feature: {{feature_name}}"
  handle: "feature-{{feature_name}}"
  labels: [feature, fullstack]
  description: |
    Add {{feature_name}} to the API and the web client.
tasktree:
  repos:
    - {repo: "git@github.com:myorg/api.git", branch: "feature/{{feature_name}}", from: "{{base_branch}}"}
    - {repo: "git@github.com:myorg/web.git", branch: "feature/{{feature_name}}", from: "{{base_branch}}"}
  symlinks:
    - {path: shared-config, target: ../shared-config}
  files:
    - path: NOTES.md
      content: |
        # {{feature_name}}

        Scratch notes for this feature. The task record holds decisions;
        use `tasktree note` for those.
    - path: check.sh
      mode: "0755"
      content: |
        #!/bin/sh
        set -e
        (cd api && npm test)
        (cd web && npm test)
  bootstrap:
    - {name: api-deps, run: npm ci, workdir: api}
    - {name: web-deps, run: npm ci, workdir: web, env: {CI: "1"}}
```

| Section | Fields |
|---|---|
| `apiVersion`, `kind` | Always `tasktree.dev/v2` and `Template`. |
| `metadata` | `name` (required), `description`. |
| `parameters` | A list of `name`, `required`, `default`, `description`. |
| `task` | `title`, `handle`, `jira`, `status`, `labels`, `description`. All are optional. |
| `tasktree.repos` | `repo` (alias or URL, required), `name`, `branch`, `from`, `mode`. They mean what `--name`, `--branch`, `--from` and `--mode` mean for [`add`](tasktrees.md#how-add-chooses-branch-and-mode). |
| `tasktree.symlinks` | `path`, `target`. Stored as symlinks. |
| `tasktree.files` | `path`, `content`, `mode` (`0644` or `0755`, default `0644`). |
| `tasktree.bootstrap` | `name`, `run`, `workdir`, `env`. They become the bootstrap steps in `.tasktree/config.yml`. |

Without a `tasktree` section, a template creates only a record. Paths in `files` and `symlinks` are relative, contain no `..`, do not start with `.git` or `.tasktree`, and do not collide with each other or with a repository name.

If no file is named `AGENTS.md`, the tasktree gets the starter `AGENTS.md`. Supply your own file with that path to replace it.

## Variables

Any string in the template may contain `{{name}}`. Names match `[a-z][a-z0-9_]*`. Every variable must be declared under `parameters`, or `validate` fails. `{{name | default:value}}` supplies a fallback at the place of use.

A value is taken from the first of these that has it:

1. A `name=value` argument on the command line.
2. The environment variable `TASKTREE_VAR_<NAME>`, with the name upper-cased.
3. The parameter's `default`.
4. The `| default:` fallback at the place of use.

```bash
tasktree new --template bugfix ticket_number=PROJ-11 repo=api
TASKTREE_VAR_REPO=api tasktree new --template bugfix ticket_number=PROJ-11
```

A parameter with `required: true` and no value makes `new` fail with exit code 2:

```console
$ tasktree new --template bugfix ticket_number=PROJ-10
error: unresolved required parameters: repo
```

A `name=value` argument for an undeclared name is also an error.

## Overriding the record

With `--template`, the title argument is optional. These options replace the rendered template values:

| Option | Effect |
|---|---|
| `"title"` (first argument, when it has no `=`) | Replaces `task.title` |
| `--handle` | Replaces `task.handle` |
| `--jira` | Replaces `task.jira` |
| `--status` | Replaces `task.status` |

The handle is `--handle`, else `task.handle`, else the Jira key, else a slug of the title. A template that sets `handle` therefore wins over `--jira`; `--jira` then only adds the reference.

`--isolated`, `--repos` and `--branch` are not allowed with `--template`. `--parent` is allowed only when the template has no `tasktree` section.

## Provenance

The first log entry of the new task records the template and the values it used:

```text
### 2026-10-13T14:27:08Z note alice@laptop {#e-...}
Created from template bugfix
base_branch=main
repo=api
ticket_number=PROJ-9
```

## Editor support

`schema/template.schema.json` describes the format. Add this first line to a template to get completion and checks in editors that use `yaml-language-server`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tinmancoding/tasktree/main/schema/template.schema.json
```

See [Editor schemas](../reference/files.md#editor-schemas).
