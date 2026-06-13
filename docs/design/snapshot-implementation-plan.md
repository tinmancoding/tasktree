# Implementation Plan: Snapshot / Restore

Status: **Implemented** · Tracks the **Snapshot/Restore — Accepted** section of
[tasktree-snapshots-and-bootstrap.md](./tasktree-snapshots-and-bootstrap.md).

All phases below are delivered and merged into the working tree. Code:
`internal/domain/snapshot.go`, `internal/gitx/snapshot.go`, `internal/snapshot/` (archive/dirty/git),
`internal/app/snapshot_service.go`, `internal/app/restore_service.go`, `internal/cli/snapshot.go`,
`internal/cli/restore.go`, `internal/cli/root.go`, `README.md`. All tests + `go vet` + `gofmt` pass;
end-to-end round-trip verified (clean, local commits, dirty edits, detached HEAD, deletions,
no-origin failure, non-empty-target refusal, version rejection, and the `snapshot -o - | restore -`
stdin/stdout pipe).

This plan delivers `tasktree snapshot` and `tasktree restore`: a single portable, self-contained
`.tar.gz` capture of a workspace's concrete state, and a deterministic restore onto a fresh
machine. Bootstrap is already shipped; restore **reuses** the bootstrap executor.

## Decisions locked (from design interview)

| # | Decision |
|---|---|
| Artifact | Single portable **`.tar.gz`**, built in a temp staging dir then packed. Self-contained: embeds a copy of `Tasktree.yml`. |
| Layout | `snapshot.yaml` (manifest) + `Tasktree.yml` + `bundles/<source>.bundle` + `dirty/<source>.tar`. |
| Dirty archive | **tar** (not zip) — preserves unix modes/symlinks; we are already inside a `.tar.gz`. |
| Snapshotter | `internal/snapshot` **free functions** + `SnapshotService` **type-switch**. No interface/registry. Non-git → no sub-snapshot (inventory-only). |
| Base SHA | `merge-base HEAD <remote-ref>` (upstream → `origin/<branch>` → `origin/HEAD`). **Always pinned** (base==head when clean). Rely-on-remote; restore verifies `HEAD==headSHA`. |
| Committed delta | `git bundle` of `base..HEAD`, **HEAD-only** in v0. |
| Dirty capture | **content** of changed + untracked paths (`git status --porcelain -z --untracked-files=all`), not diffs. `dirty.manifest` member: deletions + staged-path hint. Partial-staging → worktree content wins. `.gitignore` respected unless `--include-ignored`. |
| `remoteURL` | Read from the **on-disk `origin`**, not the spec. |
| Git edge cases | Detached HEAD recorded/restored; local-only branch → default-branch fork point; **no `origin` → hard-fail at snapshot**; submodules **out of scope**. |
| Manifest | `version: 1`; restore rejects unknown/newer major version. Non-git sources are inventory-only (name+type) for mismatch detection. |
| snapshot CLI | Root resolved from any subdir; default `./<name>-<UTC-ts>.tar.gz`; `-o <path>`; **`-o -` → stdout**; atomic temp+rename write; **hard-fail if a source is not materialized**; clean workspace valid; no `--dry-run`. |
| restore CLI | `restore <snapshot\|-> [--into <dir>] [--skip-bootstrap]`. `-` reads tar.gz from stdin. Default target `./<name>`; **refuse non-empty** (no `--force` v0). |
| restore flow | Split by type: non-git via `materialize`; git via deterministic pin path. Atomic temp→rename for the materialize phase; **bootstrap after, default-on, fail-fast/no-rollback**; **register** workspace on success. |

---

## Phase 1 — Manifest domain model & versioning

**Files:** `internal/domain/snapshot.go` (new), `internal/domain/errors.go`, tests.

1. Add manifest types:
   ```go
   const SnapshotManifestVersion = 1

   type SnapshotManifest struct {
       Version   int                    `yaml:"version"`
       CreatedAt time.Time              `yaml:"createdAt"`
       Tasktree  string                 `yaml:"tasktree"`
       Sources   []SnapshotSourceEntry  `yaml:"sources"`
   }

   type SnapshotSourceEntry struct {
       Name string                 `yaml:"name"`
       Type SourceType             `yaml:"type"`
       Git  *GitSubSnapshot        `yaml:"git,omitempty"`
   }

   type GitSubSnapshot struct {
       RemoteURL      string `yaml:"remoteURL"`
       Branch         string `yaml:"branch,omitempty"`
       Detached       bool   `yaml:"detached,omitempty"`
       BaseSHA        string `yaml:"baseSHA"`
       HeadSHA        string `yaml:"headSHA"`
       Bundle         string `yaml:"bundle,omitempty"`   // path within the tar
       Dirty          string `yaml:"dirty,omitempty"`    // path within the tar
       IncludeIgnored bool   `yaml:"includeIgnored,omitempty"`
   }
   ```
2. Add the inner dirty side-manifest type (lives inside `dirty/<source>.tar`):
   ```go
   type DirtyManifest struct {
       Deleted []string `yaml:"deleted,omitempty"` // tracked paths deleted in the worktree
       Staged  []string `yaml:"staged,omitempty"`  // re-stage hint
   }
   ```
3. Typed errors (mirroring existing `errors.go` style): `UnsupportedSnapshotVersionError{Found,Max}`,
   `SourceNotMaterializedError{Name,Path}`, `MissingOriginRemoteError{Name}`,
   `HeadMismatchError{Name,Want,Got}`, `SnapshotSourceMismatchError{...}`,
   `NonEmptyRestoreTargetError{Path}`.
4. `ValidateManifest(m SnapshotManifest) error` — version supported, names unique/non-empty,
   git entries have base+head+remoteURL.

**Tests:** round-trip marshal/unmarshal; version gate; validation table.

---

## Phase 2 — gitx primitives for snapshot/restore

**Files:** `internal/gitx/snapshot.go` (new), tests (real `git`, skip if absent).

Add thin command wrappers (pure, like existing `gitx`):

1. **Capture-side**
   - `RemoteURL(ctx, repo, name) (string, error)` → `remote get-url`; distinguish "no remote".
   - `UpstreamRef(ctx, repo) (string, error)` → `rev-parse --abbrev-ref --symbolic-full-name @{u}` (empty if none).
   - `MergeBase(ctx, repo, a, b) (string, error)`.
   - `BundleCreate(ctx, repo, outPath, revRange string) error` → `bundle create <out> <range>`
     (e.g. `base..HEAD`); detect the "empty bundle / no commits" case and signal it (no bundle).
   - `StatusPorcelainZ(ctx, repo, untrackedAll, includeIgnored bool) ([]StatusEntry, error)` →
     parse `status --porcelain=v1 -z`; classify each path: modified/added/deleted/untracked,
     staged flag, rename old→new.
   - Reuse existing `CommitSHA`, `CurrentBranch`, `CurrentFullRef`, `DefaultBranch`.
2. **Restore-side**
   - `FetchBundle(ctx, repo, bundlePath string) error` → `fetch <bundle> 'refs/*:refs/snapshot/*'`
     (or fetch by the bundle's recorded tip), bringing `base..HEAD` objects in.
   - `FetchSHA(ctx, repo, remote, sha string) error` → ensure base present from remote when the
     cache lacks it (`fetch origin <sha>` with fallback to a full fetch if the server rejects
     by-SHA fetch).
   - `CheckoutDetached(ctx, repo, sha string) error`.
   - `CreateBranchAt(ctx, repo, branch, sha string) error`.
   - `AddPaths(ctx, repo string, paths []string) error` → re-stage hint.

**Tests:** create a scratch repo via `internal/testutil/git`, exercise merge-base, bundle
create+fetch round-trip, porcelain parsing (modified/untracked/deleted/rename), detached checkout.

---

## Phase 3 — snapshot package (capture)

**Files:** `internal/snapshot/git.go`, `internal/snapshot/dirty.go`, `internal/snapshot/archive.go` (tar/gz), tests.

1. **`snapshot.Git(ctx, srcPath string, includeIgnored bool, git gitx.Client) (GitCapture, error)`**
   - Resolve `remoteURL` (hard-fail `MissingOriginRemoteError` if absent).
   - Determine `remote-ref`: upstream → `origin/<currentBranch>` → `origin/HEAD`.
   - `baseSHA = MergeBase(HEAD, remote-ref)`; `headSHA = CommitSHA(HEAD)`.
   - `branch` from `CurrentBranch` (empty ⇒ `detached=true`).
   - If `baseSHA != headSHA`: `BundleCreate(base..HEAD)` → bundle bytes (else no bundle).
   - Dirty: `StatusPorcelainZ` → collect changed+untracked content into a tar (`dirty.go`),
     build `DirtyManifest{Deleted, Staged}`; empty ⇒ no dirty.
   - Returns the sub-snapshot entry + in-memory (or temp-file) bundle/dirty payloads.
2. **`dirty.go`** — `BuildDirtyTar(srcPath string, entries []gitx.StatusEntry) ([]byte/tempfile, DirtyManifest, error)`:
   walk changed+untracked paths, write file content + mode into a tar, record deletions/staged.
   Skip ignored unless `includeIgnored` (porcelain already excludes ignored by default).
3. **`archive.go`** — `Pack(manifest, specBytes, perSourcePayloads) -> .tar.gz` writer and the inverse
   `Open(reader) -> (manifest, spec, payload accessor)`. Streams to an `io.Writer` (supports stdout).

**Tests:** clean repo → pins only; local commits → bundle present + correct range; dirty (modified,
new, deleted, rename, mode change) → tar content + manifest correct; `--include-ignored` toggles
ignored files; detached HEAD path; missing origin → error.

---

## Phase 4 — SnapshotService + snapshot CLI

**Files:** `internal/app/snapshot_service.go` (new), `internal/cli/snapshot.go` (new),
`internal/cli/root.go` (deps), tests.

1. **`SnapshotService`** (inject `metadata.Store`, `gitx.Client`):
   - Resolve root (`fsx.ResolveTasktreeRoot`), `store.Load`.
   - For each source: verify dir exists (`SourceNotMaterializedError` if missing). `switch type`:
     `git` → `snapshot.Git`; others → inventory-only entry.
   - Assemble `SnapshotManifest`, embed rendered `Tasktree.yml` bytes, `Pack` to an `io.Writer`.
   - `Run(ctx, start string, opts SnapshotOptions) (SnapshotResult, error)` where `opts` carries
     `Output io.Writer` (or path) + `IncludeIgnored`.
2. **CLI** `tasktree snapshot [-o <path>] [--include-ignored]`:
   - Default output `./<name>-<UTC-ts>.tar.gz`; `-o -` → `cmd.OutOrStdout()`.
   - File output written atomically (temp + rename via `fsx`); stdout streamed directly.
   - Wire into `defaultDependencies()` / `dependencies` struct.

**Tests:** service-level (fake git) for source iteration + missing-source failure + inventory of
non-git; CLI behavior for default name, `-o -` stdout, `--include-ignored` plumbing.

---

## Phase 5 — restore package + RestoreService

**Files:** `internal/snapshot/restore.go`, `internal/app/restore_service.go` (new), tests.

1. **`RestoreService`** (inject `metadata.Store`, `cache.Manager`, `gitx.Client`, `BootstrapRunner`):
   - `Open` the tarball (path or stdin reader) → manifest (version gate) + embedded spec.
   - Determine target: `--into` or `./<manifest.Tasktree>`. **Refuse non-empty** target.
   - Create a temp staging dir on the **same filesystem** as the target's parent.
   - Write `Tasktree.yml` into staging. For each source:
     - non-git → `materialize.*` from spec into staging.
     - git → deterministic path: cache clone → set `origin` → ensure `baseSHA`
       (`FetchSHA` if needed) → `FetchBundle` if present → recreate branch (`CreateBranchAt`) or
       `CheckoutDetached` at `headSHA` → **verify `HEAD==headSHA`** (`HeadMismatchError`) →
       unpack `dirty/<source>.tar`, apply deletions, `AddPaths` staged hint.
   - `os.Rename` staging → target (all-or-nothing); cleanup temp on any failure.
   - Register workspace in the registry.
   - Unless `--skip-bootstrap` or no steps: run `bootstrap.Run(ctx, target, steps, ...)` on the
     **final** dir (fail-fast, no rollback — same contract as apply).
2. **`restore.go`** — `UnpackDirtyTar(targetSrc string, tarReader)` applies content + deletions.

**Tests:** end-to-end with scratch repos: clean restore (pins only), restore with bundle commits,
restore with dirty (modified/new/deleted) verifying exact tree, detached restore, head-mismatch
failure, non-empty target refusal, bootstrap-runs / `--skip-bootstrap`, registry registration,
temp cleanup on mid-restore failure.

---

## Phase 6 — restore CLI, docs, schema notes

**Files:** `internal/cli/restore.go` (new), `internal/cli/root.go`, `README.md`, `mkdocs.yml`.

1. CLI `tasktree restore <snapshot|-> [--into <dir>] [--skip-bootstrap]`:
   - `-` → read tar.gz from `cmd.InOrStdin()`.
   - Wire `RestoreService` into deps; map errors via `formatError`; non-zero exit on failure.
2. **README** snapshot/restore section; `mkdocs.yml` nav if a dedicated doc page is warranted.
3. No `Tasktree.yml` schema change (snapshot manifest is a separate artifact, not user-authored);
   optionally publish a `schema/snapshot.schema.json` for tooling — **deferred** unless useful.

---

## Sequencing & verification

Order: **1 → 2 → 3 → 4** (capture vertical slice ships first and is independently useful), then
**5 → 6** (restore). Each phase: `make build` + `go test ./...` + `go vet` + `gofmt`.

Final acceptance:
- `tasktree snapshot` on a clean workspace → valid pins-only `.tar.gz`.
- `tasktree snapshot` with local commits + dirty edits → bundle + dirty tars; `-o -` streams.
- `tasktree snapshot` hard-fails when a declared source is not materialized.
- `tasktree restore <snap> --into <fresh>` reproduces the exact working tree (verified `HEAD`),
  refuses a non-empty target, runs bootstrap by default, registers the workspace.
- `tasktree snapshot -o - | tasktree restore - --into <fresh>` round-trips with no temp file.
- Restore of a snapshot with an unknown manifest version fails clearly.

## Open/deferred (not in this increment)

- Multi-branch (all local refs) bundle capture.
- True index fidelity (separate index vs worktree blobs for partially staged paths).
- Self-contained-bundle fallback when `origin` is unavailable / base unfetchable.
- Submodule capture/restore.
- `--force` overwrite of a non-empty restore target; in-place restore.
- `snapshot --dry-run`; `schema/snapshot.schema.json`.
- Windows support (consistent with bootstrap's Unix-only v0).
