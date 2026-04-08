# Tasktree Declarative Schema Design

## Summary

This document defines the new declarative file format for Tasktree workspaces,
replacing the previous `.tasktree.toml` format. It covers the design philosophy,
the schema for `Tasktree.yml`, the source type taxonomy, and the full migration
plan for aligning the Go codebase to this design.

---

## Motivation

The original `.tasktree.toml` served well as a metadata file, but it has two
limitations that block future evolution:

1. **Implicit git-only assumption.** Every field in `[[repos]]` is Git-specific.
   Adding a different source type (a downloaded file, an archive, a local
   symlink) would require adding a new top-level array and duplicating all the
   surrounding machinery.

2. **Non-portable by default.** Fields like `checkout` (user intent) and
   `resolved_ref`, `commit` (what the tool resolved) live in the same struct.
   Copying the file to a new machine carries stale resolved state that is
   meaningless there. The new design keeps `Tasktree.yml` as pure intent — always
   safe to copy and share.

The new design is inspired by Kubernetes resource manifests.

---

## Design Principles

### 1. `Tasktree.yml` is pure desired state

The file named `Tasktree.yml` at the root of a tasktree describes **what you
want**, not what was resolved at materialization time. It contains no commit
SHAs, no resolved refs, no timestamps written by the tool. It is always safe to
copy to a new directory and run `tasktree apply`.

The tool writes to `Tasktree.yml` only to add or remove entries in
`spec.sources` (on `tasktree add` / `tasktree remove`). It never writes resolved
state back into the file.

### 2. Resolved state is ephemeral

Commit SHAs, resolved refs, and materialization timestamps are not persisted to
any file. They live only in the Git checkouts themselves and are queried live by
`tasktree status`. This is consistent with how the existing `status` command
already works — it reads live Git state, not cached metadata.

If pinned-commit reproducibility becomes a requirement in future, a lock file
can be introduced at that point. For now, "apply on a new machine" means
"re-resolve the declared branches/tags from their current remote state," which
is the correct default for a developer workspace tool.

### 3. Kubernetes-style structure

`Tasktree.yml` uses the same four top-level keys that Kubernetes resources use:

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree
metadata: ...
spec: ...
```

This makes the format recognizable to practitioners who know Kubernetes and
gives the tool a clean versioning path (`tasktree.dev/v2`, etc.) without
breaking changes.

### 4. Generic `sources` instead of `repos`

`spec.sources` replaces `[[repos]]`. A source is anything that produces content
in the workspace directory: a Git clone, a downloaded file, an archive, inline
content, or a local path. Each source has:

- `name`: logical identifier, used in CLI commands
- `type`: discriminator (`git`, `http`, `archive`, `static`, `local`)
- `path`: relative path inside the tasktree where it materializes
- A type-specific block (`git:`, `http:`, etc.)

Only `git` is implemented in v1. The other types are defined in the schema as a
forward-looking contract.

### 5. Hard cutover from `.tasktree.toml`

The old format is not supported in parallel. A `tasktree migrate` command
converts existing `.tasktree.toml` files to `Tasktree.yml`. Resolved state
fields (`resolved_ref`, `commit`) from the old format are intentionally
discarded — they are stale metadata that should not travel with the spec.

---

## File Naming

| File | Owned by | Commit to VCS? |
|---|---|---|
| `Tasktree.yml` | User (tool appends/removes entries) | Yes — it is the source of truth |

`Tasktree.yml` is a capitalized proper noun, like `Makefile` or `Dockerfile`.
It is not a hidden dotfile because it is a first-class project artifact.

---

## `Tasktree.yml` — Full Reference

The JSON Schema is at `schema/tasktree.schema.json`. Below is a full annotated
example covering all currently defined source types.

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree

metadata:
  name: feature-payments
  description: "Workspace for the Q2 payments feature work"
  createdAt: "2026-03-25T12:00:00Z"   # set by tasktree init, do not edit
  labels:
    team: payments
    ticket: PAY-1234

spec:
  sources:

    # --- git source (v1, fully implemented) ---
    - name: api
      type: git
      path: api                            # optional, defaults to name
      git:
        url: "git@github.com:myorg/api.git"
        ref: main                          # optional, defaults to remote default branch
        branch: feature/payments          # optional: local branch to create/track

    - name: web
      type: git
      git:
        url: "git@github.com:myorg/web.git"
        ref: "v1.4.0"                      # tag checkout, detached HEAD

    - name: contracts
      type: git
      git:
        url: "git@github.com:myorg/contracts.git"
                                           # no ref, no branch -> remote default branch

    # --- http source (future) ---
    - name: shared-config
      type: http
      path: config/base.json
      http:
        url: "https://example.com/config/base.json"
        sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

    # --- archive source (future) ---
    - name: proto-definitions
      type: archive
      path: proto
      archive:
        url: "https://github.com/myorg/protos/archive/refs/tags/v2.1.0.tar.gz"
        sha256: "abc123..."
        stripComponents: 1

    # --- static source (future) ---
    - name: dev-env-stub
      type: static
      path: .env.development
      static:
        mode: "0600"
        content: |
          DATABASE_URL=postgres://localhost:5432/payments_dev
          REDIS_URL=redis://localhost:6379

    # --- local source (future) ---
    - name: shared-scripts
      type: local
      path: scripts
      local:
        sourcePath: /home/user/common-scripts
        copy: false                        # false = symlink (default), true = copy
```

---

## Source Type Taxonomy

| Type | Status | Description | Key spec fields |
|---|---|---|---|
| `git` | **v1 — implemented** | Full Git repository clone via tasktree cache | `url`, `ref`, `branch` |
| `http` | future | Single-file HTTPS download with optional digest verification | `url`, `sha256`, `headers` |
| `archive` | future | HTTPS tarball/zip download + extraction | `url`, `sha256`, `format`, `stripComponents` |
| `static` | future | Inline file content embedded in Tasktree.yml | `content`, `mode` |
| `local` | future | Symlink or copy from a local filesystem path | `sourcePath`, `copy` |

The schema explicitly enumerates the known types. The CLI returns a clear error
for any unknown type so invalid configurations are caught early.

---

## Command Model Changes

### `tasktree init [path]`

**Unchanged behavior.** Creates `Tasktree.yml` (instead of `.tasktree.toml`)
with an empty `spec.sources` list.

### `tasktree add <url-or-alias> [flags]`

**Unchanged UX.** Internally appends a `spec.sources` entry to `Tasktree.yml`
and materializes the source (clone + checkout). No resolved state is written
back to the file.

### `tasktree apply` *(new command)*

Reads `Tasktree.yml` and ensures the workspace matches the spec. For each source
not yet materialized, it performs the necessary action (clone, download, write
static file, etc.). Supports `--dry-run`.

This is the key portability command: copy `Tasktree.yml` to a new folder, run
`tasktree apply`, and the workspace is reproduced.

### `tasktree remove <name>`

**Unchanged behavior.** Removes the source entry from `spec.sources` in
`Tasktree.yml` and removes the materialized content from disk.

### `tasktree status`

**Unchanged behavior.** Reads `spec.sources` from `Tasktree.yml` and queries
live state from each Git checkout. No persisted resolved state is consulted.

### `tasktree migrate` *(new command)*

Converts an existing `.tasktree.toml` to `Tasktree.yml`:

1. Read `.tasktree.toml`
2. Map `[[repos]]` entries → `spec.sources` of type `git` (only `url`, `ref`,
   `branch` are carried over; `resolved_ref` and `commit` are discarded)
3. Write `Tasktree.yml`
4. Rename `.tasktree.toml` → `.tasktree.toml.bak`
5. Print a confirmation summary

### `tasktree snapshot` / `tasktree export` *(future — see below)*

See the **Future: Snapshot** section.

---

## Future: Snapshot

A `tasktree snapshot` (or `tasktree export`) command is planned to address a
different, more demanding use case: capturing the **exact current state** of a
workspace — including uncommitted changes, untracked files, and unpushed local
branches — and packaging it into a portable asset that can be imported on
another machine to reproduce the workspace byte-for-byte.

This is fundamentally different from sharing `Tasktree.yml`:

| | `Tasktree.yml` | Snapshot asset |
|---|---|---|
| What it captures | Intent (repos, branches) | Exact state (commits + uncommitted work) |
| Uncommitted changes | No | Yes |
| Portable to new machine | Yes, re-resolves from remote | Yes, self-contained |
| Human-readable | Yes | No (binary bundle) |
| Suitable for VCS | Yes | No |

The snapshot output is not a YAML file. It is closer to a structured archive
containing a `Tasktree.yml` (the spec) plus per-repository Git bundles
(`git bundle`) and patch files for any uncommitted changes. The exact format
will be designed when the command is implemented.

---

## Go Codebase Migration Plan

This section describes every change needed to align the Go implementation with
the new schema. The changes are grouped by package.

### Layer 1 — `internal/domain`

**`domain/tasktree.go`**

Replace `TasktreeFile` and `RepoSpec` with:

```go
// TasktreeSpec is the user-authored declarative workspace file (Tasktree.yml).
type TasktreeSpec struct {
    APIVersion string        `yaml:"apiVersion"`
    Kind       string        `yaml:"kind"`      // must be "Tasktree"
    Metadata   SpecMetadata  `yaml:"metadata"`
    Spec       WorkspaceSpec `yaml:"spec"`
}

type SpecMetadata struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    CreatedAt   time.Time         `yaml:"createdAt,omitempty"`
    Labels      map[string]string `yaml:"labels,omitempty"`
}

type WorkspaceSpec struct {
    Sources []SourceSpec `yaml:"sources"`
}

type SourceSpec struct {
    Name string         `yaml:"name"`
    Type SourceType     `yaml:"type"`
    Path string         `yaml:"path,omitempty"`
    Git  *GitSourceSpec `yaml:"git,omitempty"`
    // future: HTTP, Archive, Static, Local
}

type SourceType string

const (
    SourceTypeGit     SourceType = "git"
    SourceTypeHTTP    SourceType = "http"
    SourceTypeArchive SourceType = "archive"
    SourceTypeStatic  SourceType = "static"
    SourceTypeLocal   SourceType = "local"
)

type GitSourceSpec struct {
    URL    string `yaml:"url"`
    Ref    string `yaml:"ref,omitempty"`
    Branch string `yaml:"branch,omitempty"`
}
```

**`domain/errors.go`**

- Update all error messages referencing `.tasktree.toml` to `Tasktree.yml`.
- Add `UnknownSourceTypeError`, `MissingSourceSpecError`.

**`domain/tasktree.go` constants**

```go
const (
    SpecFileName   = "Tasktree.yml"
    LegacyFileName = ".tasktree.toml"

    APIVersion   = "tasktree.dev/v1"
    KindTasktree = "Tasktree"
)
```

---

### Layer 2 — `internal/metadata`

**`metadata/store.go`**

Rename and simplify to a single `SpecStore` that reads/writes `Tasktree.yml`.
Switch from `go-toml/v2` to `gopkg.in/yaml.v3` (already a dependency). Atomic
writes via `fsx.AtomicWriteFile` are retained.

```go
type SpecStore struct{}

func (s SpecStore) Path(root string) string
func (s SpecStore) Load(root string) (domain.TasktreeSpec, error)
func (s SpecStore) Save(root string, spec domain.TasktreeSpec) error
```

---

### Layer 3 — `internal/fsx`

**`fsx/paths.go`**

`ResolveTasktreeRoot` currently searches for `domain.MetadataFileName`
(`.tasktree.toml`). Change it to search for `domain.SpecFileName`
(`Tasktree.yml`). Also check for `domain.LegacyFileName` and surface a clear
migration error if found:

```
Found legacy .tasktree.toml. Run `tasktree migrate` to convert to Tasktree.yml.
```

---

### Layer 4 — `internal/app`

**`app/init_service.go`**

Construct `domain.TasktreeSpec` with the correct `apiVersion`, `kind`,
metadata, and an empty `spec.sources` list. Use `SpecStore.Save`.

**`app/add_service.go`**

`AddOptions` stays the same shape (RepoURL, Branch, From, Name). After
successful checkout, append a `domain.SourceSpec` (type: `git`) to
`spec.sources` and save via `SpecStore`. The `domain.RepoSpec` type is replaced
by `domain.SourceSpec` + `domain.GitSourceSpec`. No resolved state is written.

**`app/remove_service.go`**

Remove the matching entry from `spec.sources` and save via `SpecStore`.

**`app/list_service.go`**

Read from `spec.sources` instead of `TasktreeFile.Repos`.

**`app/status_service.go`**

Unchanged logic. Read `spec.sources` for the list of repos; live Git state is
queried as before.

**`app/migrate_service.go`** *(new)*

```go
type MigrateService struct {
    specStore metadata.SpecStore
}

// Run reads .tasktree.toml, produces Tasktree.yml (discarding resolved state),
// renames .tasktree.toml to .tasktree.toml.bak.
func (s MigrateService) Run(root string) (MigrateResult, error)
```

**`app/apply_service.go`** *(new, future)*

Reconciles desired state (spec) with the actual workspace. For each source in
`spec.sources` not yet materialized on disk, performs the appropriate action.

---

### Layer 5 — `internal/cli`

**`cli/init.go`**

No UX change. Help text and output messages refer to `Tasktree.yml` instead
of `.tasktree.toml`.

**`cli/migrate.go`** *(new)*

Implements `tasktree migrate`. Optional path argument (defaults to CWD). Prints
a per-source summary of what was carried over and what was discarded.

**`cli/apply.go`** *(new, future)*

Implements `tasktree apply [--dry-run]`.

**All other CLI files**

`RepoSpec` / `TasktreeFile` references become `SourceSpec` / `TasktreeSpec`.
Output table column `CHECKOUT` becomes `REF`.

---

### Layer 6 — `internal/registry`

No change required. The global registry
(`~/.local/state/tasktree/registry.toml`) stores only tasktree paths and names.
It can remain TOML.

---

### Layer 7 — `internal/output`

Table renderers update field names: `URL`, `Ref`, `Branch` (from
`GitSourceSpec`) instead of `URL`, `Checkout`, `Branch` (from the old
`RepoSpec`). The change is mechanical.

---

## Migration Command UX

```text
$ tasktree migrate
Found .tasktree.toml in current directory.

Converting to Tasktree.yml...

  api          git  git@github.com:DiligentCorp/sfs.git  (branch: poc-bcloud-integration)
  sfs-aws-iac  git  git@github.com:DiligentCorp/sfs-aws-iac.git  (branch: poc-bcloud-integration)
  sfs-aws-gitops  git  git@github.com:DiligentCorp/sfs-aws-gitops.git  (branch: poc-bcloud-integration)

Note: resolved_ref and commit fields are not carried over.
      Live state is always queried from the Git checkouts directly.

Written: Tasktree.yml
Renamed: .tasktree.toml → .tasktree.toml.bak

Migration complete. Review Tasktree.yml and commit it to version control.
```

---

## Backward Compatibility Policy

| Scenario | Behavior |
|---|---|
| Directory has `Tasktree.yml` | Normal operation |
| Directory has `.tasktree.toml` | Error: "Found legacy .tasktree.toml. Run `tasktree migrate` to convert." |
| Directory has both | `Tasktree.yml` takes precedence; warn about leftover `.tasktree.toml` |
| Directory has neither | Normal "not inside a tasktree" error |

---

## File Placement in the Repository

```
schema/
  tasktree.schema.json         # JSON Schema for Tasktree.yml

docs/
  DECLARATIVE_SCHEMA.md        # This document
  PRD.md
  SYSTEM_DESIGN.md
  IMPLEMENTATION_PLAN.md
```

Editor integrations (VS Code, JetBrains YAML plugin, etc.) can be pointed at
`schema/tasktree.schema.json` for inline validation and autocompletion.

---

## Example: Full Reproduced Workspace

Given a `Tasktree.yml` committed to a shared repository or sent to a teammate:

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree
metadata:
  name: feature-payments
spec:
  sources:
    - name: api
      type: git
      git:
        url: "git@github.com:myorg/api.git"
        ref: main
        branch: feature/payments
    - name: web
      type: git
      git:
        url: "git@github.com:myorg/web.git"
        ref: "v1.4.0"
```

A new developer reproduces it on any machine:

```bash
mkdir ~/ws/feature-payments
cp Tasktree.yml ~/ws/feature-payments/
cd ~/ws/feature-payments
tasktree apply
```

No resolved state, no stale commits, no machine-specific metadata — just intent.
