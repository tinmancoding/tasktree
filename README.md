# tasktree

tasktree manages tasks and the multi-repository workspaces for them in one command-line tool. A task is a Markdown record with a status, references and a log. Its workspace is a directory with a full clone of each repository you work on. Both live in git, so one `~/.tasktree` directory holds everything, and a plain git remote syncs it between machines.

## Install

Requires git 2.40 or later, on Linux or macOS.

```bash
go install github.com/tinmancoding/tasktree/v2/cmd/tasktree@latest
```

Or clone this repository and run `make install`, which installs `tasktree` and the `tt` and `tt2` symlinks into `~/.local/bin`. Release archives are on the [releases page](https://github.com/tinmancoding/tasktree/releases). See [Installation](https://tinmancoding.github.io/tasktree/getting-started/installation/) for details.

## Quickstart

```bash
tasktree setup --author alice --machine laptop
tasktree new "Fix login redirect" --jira PROJ-123
tasktree add git@github.com:myorg/api.git --branch fix/PROJ-123 -t PROJ-123
cd "$(tasktree open PROJ-123)"
# work in api/ with plain git, then:
tasktree commit -m "Record the fix"
tasktree note "Root cause: the return URL was decoded twice"
tasktree mark done
```

`mark done` deletes the checkout, after it checks that nothing would be lost. The task and its history stay. The [quickstart](https://tinmancoding.github.io/tasktree/getting-started/quickstart/) walks through this with real output.

## Features

- **Tasks as files.** Each task is a Markdown record with typed fields, an editable description and an append-only log. `tasktree history` is the git history of the record.
- **Workspaces as tasktrees.** A task has zero or one tasktree: repositories with a mode (`follow`, `work` or `pin`), files, symlinks and bootstrap steps. It is checked out where you want it, and can be defined without a checkout.
- **Git is the only store.** Records and tasktrees live in a hidden git repository. Every repository in a checkout is a normal full clone, and git works inside it as always.
- **Safe by default.** Closing or removing something checks for uncommitted changes, unpushed commits and stashes first. Sync never rebases or resets your branches.
- **Subtasks.** Share the parent's tasktree, or fork an isolated one from the parent's current commit and send it back through a pull request.
- **Sync between machines.** `tasktree sync` merges records field by field, moves tasktrees and reports conflicts. `resolve` settles diverged tasktrees.
- **Templates.** Create a task and its repositories, files and bootstrap steps from a short YAML file.
- **Made for coding agents.** `--json` output, stable exit codes, a generated `AGENTS.md`, per-agent authorship.
- **Recovery tools.** `doctor` finds and repairs broken checkouts. `move` relocates a checkout atomically.

## Documentation

The documentation is at <https://tinmancoding.github.io/tasktree/>.

- [Getting started](https://tinmancoding.github.io/tasktree/getting-started/installation/): install, quickstart, importing V1 workspaces
- [Concepts](https://tinmancoding.github.io/tasktree/concepts/model/): the model, task records, tasktrees, subtasks, sync, templates
- [Guides](https://tinmancoding.github.io/tasktree/guides/agents/): working with agents, two machines, doctor and recovery
- [Reference](https://tinmancoding.github.io/tasktree/reference/exit-codes/): commands, exit codes, JSON output, configuration, environment, files
- [Design](https://tinmancoding.github.io/tasktree/design/tasktree-v2/): the decisions behind V2

The same files are in [`docs/`](docs/).

## Migrating from V1

V1 kept a workspace as a folder with a `Tasktree.yml`. `tasktree import` adopts those folders in place, without recloning, and rolls back if anything differs. Read [Importing V1 workspaces](https://tinmancoding.github.io/tasktree/getting-started/importing-v1/) before you replace the V1 binary. The V1 documentation stays on the [`v1` branch](https://github.com/tinmancoding/tasktree/tree/v1/docs).

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

The CLI reference in `docs/reference/cli/` is generated from the command definitions. After you change a command, flag or help text, regenerate it, or the test suite fails:

```bash
make docs-cli
```

To preview the documentation site, install `mkdocs-material` and run `mkdocs serve`. `mkdocs build --strict` fails on broken links.

Templates for your own use are in [`examples/templates/`](examples/templates/). JSON Schemas for records, tasktree config and templates are in [`schema/`](schema/).

## Releases

Commits follow [Conventional Commits](https://www.conventionalcommits.org/). [release-please](https://github.com/googleapis/release-please) keeps a release pull request open on `main`. Merging it tags the release, updates `CHANGELOG.md` and starts GoReleaser, which attaches the archives to the GitHub release.

## License

See [LICENSE](LICENSE).
