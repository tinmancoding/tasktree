# Files

## The `~/.tasktree` directory

`~/.tasktree` (or `$TASKTREE_HOME`) holds everything tasktree owns except the clone cache.

```text
~/.tasktree/
  config.yml            machine configuration
  local.yml             this machine's checkouts and focus; never synced
  local.yml.lock        lock file for local.yml
  aliases.yml           repository aliases
  aliases.yml.lock      lock file for aliases.yml
  templates/            your templates (*.yml, *.yaml)
  git/                  the tasks repository: a non-bare git repository
    .git/tasktree.lock  the write lock
    tasks/<id>.md       one record per task, on the main branch
  clones/
    <id>.git/           hidden git directory of the checkout of task <id>
      modules/<name>/   git directory of each repository in that checkout
  imports/<id>/         journal of a V1 import in progress; exists only while it runs
  imports.lock          lock file for imports
```

Everything here except `aliases.yml` and `templates/` is created by tasktree. Back up the whole directory. If a remote is configured, the records and tasktrees are also in the remote, but `local.yml`, `aliases.yml` and `templates/` are not.

| Path | Synced | Notes |
|---|---|---|
| `config.yml` | No | See [Configuration](configuration.md). |
| `local.yml` | No | `checkouts.<id>.path` and `checkouts.<id>.focus`. |
| `aliases.yml` | No | `repos:` list with `url` and `aliases`. |
| `templates/` | No | See [Templates](../concepts/templates.md). |
| `git/` | Yes, through the remote | The tasks repository. Manual edits are allowed; see below. |
| `clones/` | No | Hidden git directories. Do not edit them. |

### `local.yml`

```yaml
checkouts:
    c9sezd:
        path: /home/alice/work/tasks/PROJ-123
        focus: p3n8wd
```

`path` is the checkout directory. `focus` is optional and holds the id of the focused subtask. The file is rewritten under a lock and atomically.

### `aliases.yml`

```yaml
repos:
    - url: git@github.com:myorg/api.git
      aliases:
        - api
        - myorg-api
```

`tasktree add` registers `<name>` and `<owner>-<name>` for every URL. Manage the file with `tasktree alias add|rm|list` or edit it by hand. If `aliases.yml` does not exist when you run `setup`, V1 aliases are copied into it.

### The tasks repository

`git/` is a normal git repository, so `git -C ~/.tasktree/git log` works. `main` holds `tasks/<id>.md`. The branches `task/<id>` hold tasktrees. You may edit and commit task files there. While the repository has uncommitted changes, tasktree refuses to write, and `doctor` reports `hub-dirty`.

### The cache

The cache of bare clones is not in `~/.tasktree`. It is in `$TASKTREE_CACHE_DIR`, or in the user cache directory under `tasktree/repos` (`~/Library/Caches/tasktree/repos` on macOS, `~/.cache/tasktree/repos` on Linux). Each repository is a directory named after the SHA-256 of its URL. It is safe to delete.

## A checkout

```text
<tasksRoot>/<handle>/
  .tasktree/
    config.yml       marker and bootstrap steps; committed
    .gitignore       contains "task.md"; committed
    task.md          generated copy of the task record; ignored
  .gitmodules        repositories, branches and modes; committed
  AGENTS.md          starter rules for agents; committed, yours after creation
  <repo>/            a full git clone for each repository
  ...                your own files and symlinks; committed by `tasktree commit`
```

There is no `.git` in the root. Its git directory is `~/.tasktree/clones/<id>.git`. Each `<repo>/.git` is a file `gitdir: /home/alice/.tasktree/clones/<id>.git/modules/<repo>`.

`.tasktree/config.yml`:

```yaml
apiVersion: tasktree.dev/v2
kind: TasktreeConfig
taskId: x3kjpk
bootstrap:
    - name: deps
      run: npm ci
      workdir: api
```

A bare tasktree has only the first three lines.

## Commit messages

tasktree writes fixed commit messages, so `git log` is readable.

**Tasks repository, branch `main`:**

| Message | Written by |
|---|---|
| `setup tasktree` | `setup` |
| `task(<id>): create "<title>"` | `new` |
| `task(<id>): create from template <name>` | `new --template` |
| `task(<id>): import from V1` | `import` |
| `task(<id>): status <old> → <new>` | `mark` |
| `task(<id>): <kind> added`, with the kind `note`, `comment`, `decision` or `ref` | `note` |
| `task(<id>): description updated`, `title updated`, `handle updated`, `parent set to <id>`, `parent cleared` | `info edit`, `info set` |
| `task(<id>): conflict acknowledged: <field>` | `info ack` |
| `task(<id>): ref added`, `ref removed` | `ref` |
| `task(<id>): labels added`, `labels removed` | `label` |
| `sync: merge <machine>` | `sync`, when `main` diverged |

**Task branches `task/<id>`:**

| Message | Written by |
|---|---|
| `tasktree(<id>): create` | `open`, when the task had no tasktree |
| `tasktree(<id>): create from template <name>` | `new --template` |
| `tasktree(<id>): add <name>` | `add` |
| `tasktree(<id>): remove <name>` | `remove` |
| `tasktree(<id>): mode <name> <mode>` | `mode` |
| `tasktree(<id>): bootstrap add <step>`, `bootstrap rm <step>` | `bootstrap add`, `bootstrap rm` |
| `tasktree(<id>): checkpoint` or your message | `commit` |
| `tasktree(<id>): close: <status>` | `mark done`, `mark dropped` |
| `tasktree(<id>): sync update <name> <old>→<new>, ...` | `sync`, for repositories that moved |
| `tasktree(<id>): fork from <parent handle>` | An isolated subtask's first commit |
| `tasktree(<id>): import from V1` | `import` |
| `tasktree(<id>): resolve merge`, `resolve take local`, `resolve take remote` | `resolve` |

**Identity.** tasktree commits with your git `user.name` and `user.email` when git has them. If not, it passes the `author` and `machine` config keys for that one commit (`alice <alice@laptop>`) and never writes them to a git config.

## Editor schemas

The repository ships JSON Schemas in `schema/`:

| File | Describes |
|---|---|
| `schema/task-record.schema.json` | The frontmatter of a task record |
| `schema/tasktree-config.schema.json` | `.tasktree/config.yml` of a tasktree |
| `schema/template.schema.json` | A template |

Add a modeline to a YAML file to get completion and diagnostics in an editor that uses [`yaml-language-server`](https://github.com/redhat-developer/yaml-language-server) (VS Code with the YAML extension, Neovim, Helix, Zed and others):

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tinmancoding/tasktree/main/schema/template.schema.json
apiVersion: tasktree.dev/v2
kind: Template
```

Inside a clone of the repository, a relative path works too: `# yaml-language-server: $schema=../../schema/template.schema.json`. The files in `examples/templates/` do that.

For `.tasktree/config.yml` use `tasktree-config.schema.json`. For a task record, map the schema to the frontmatter in your editor's settings, for example in VS Code:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/tinmancoding/tasktree/main/schema/tasktree-config.schema.json": "**/.tasktree/config.yml"
  }
}
```

Task records are Markdown files with a YAML header, so most editors do not validate the header against a schema. The record schema is mainly for tools that parse the frontmatter.
