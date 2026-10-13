# Design: Bootstrap & Snapshot/Restore

Status: **Bootstrap — Accepted** · **Snapshot/Restore — Accepted**
Layer: **tasktree core**

This document specifies two new primitives in tasktree core:

1. **Bootstrap scripts** — idempotent setup steps run after sources materialize (on `apply`).
2. **Snapshot / restore** — a lightweight, portable capture of a workspace's concrete state.

Both are general-purpose features useful to anyone working with a tasktree locally. They are
also the foundation that [tasktree-engine](./tasktree-engine.md) builds on, but they carry no
dependency on it.

**Delivery order:** bootstrap ships first as a standalone, independently-releasable increment;
snapshot/restore follows as a second increment. They share almost no code, so splitting carries no
rework cost, and restore can later reuse the apply+bootstrap lifecycle that bootstrap establishes.
The implementation plan for bootstrap lives in
[bootstrap-implementation-plan.md](./bootstrap-implementation-plan.md).

## Motivation

tasktree today only *materializes desired state*: it clones/downloads sources declared in
`Tasktree.yml` and never runs anything. Two capabilities are missing:

- After sources land, a workspace usually needs **setup** (`npm install`, `go mod download`,
  generate local config). Today this is manual and undocumented per workspace.
- There is no way to **capture and reproduce** a workspace's *concrete* working state — the
  exact commits and uncommitted edits — on another machine or at a later time. `Tasktree.yml` is
  deliberately pure desired state (no resolved SHAs), so it cannot represent "where I actually am
  right now."

## Design principle: keep `Tasktree.yml` pure

`Tasktree.yml` remains **pure declarative desired state** — no resolved SHAs, no machine-specific
paths, safe to commit, idempotent `apply`. The two features here respect that line:

- **Bootstrap = environment convergence.** Idempotent, deterministic, reproducible. Running it is
  conceptually part of "materializing the desired state," the same as cloning a repo. It therefore
  lives *in* `Tasktree.yml`. If a step is not idempotent/reproducible, it does not belong in
  bootstrap — it belongs in a [tasktree-engine](./tasktree-engine.md) `Task`.
- **Snapshot = the concrete frozen instant.** It is the *complement* to the spec: the one place
  where resolved base SHAs and dirty edits are allowed to live. It is a separate artifact, never
  written back into `Tasktree.yml`.

---

## 1. Bootstrap

### Spec

A new optional workspace-level `spec.bootstrap`: an **ordered** list of steps run **after all
sources are materialized**.

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
        url: git@github.com:myorg/api.git
        branch: feature/payments
  bootstrap:
    - name: install-api-deps
      run: npm ci
      workdir: api
    - name: generate-config
      run: ./scripts/gen-local-config.sh
      env:
        ENVIRONMENT: local
```

| Field | Description |
|---|---|
| `name` | Identifier for the step (used in logs/output). |
| `run` | Shell command to execute. |
| `workdir` | Working directory relative to the workspace root. Defaults to the root. |
| `env` | Optional extra environment variables for this step. |

### Execution contract

- **Scope:** workspace-level only. There is no per-source `setup` hook — keep it simple. A step
  that needs a particular source uses `workdir`.
- **When:** after *all* sources are materialized, on **every `apply`**. Bootstrap is an
  **unconditional phase**: it runs regardless of whether any source was (re)materialized or all
  sources were skipped-as-present. There is no coupling between source-skip status and whether
  bootstrap runs. (Improving source-level apply idempotency/refresh is **out of scope** here and
  tracked separately.)
- **Idempotency:** steps **must** be idempotent. Bootstrap runs every apply with **no completion
  marker** and no hidden state — the honest model, at the cost of re-running setup each time. (If
  this becomes painful, a future opt-in cache may be considered, but the default never relies on
  hidden state.)
- **Ordering:** steps run sequentially in declared order.
- **Failure:** **fail-fast.** A non-zero exit aborts `apply` with a clear error naming the step,
  its `workdir`, and the child **exit code** (preserved/propagated, not flattened); `apply` exits
  non-zero. **No rollback** — partial filesystem state (materialized sources, files written by
  earlier steps) is left in place. Recovery is fix-the-cause and re-run `apply`; manual cleanup if
  needed. Earlier steps being idempotent makes the retry cheap.

### Execution semantics

- **Shell:** each step runs as `sh -c "<run>"` (POSIX `sh`, not `bash`, for portability and to
  keep the core lean). Steps needing `bash` features must invoke it explicitly or rely on the
  engine's runtime image.
- **Environment:** the child inherits tasktree's environment, with the step's `env` map overlaid
  on top (**step `env` wins** on key collisions — plain overlay, no special-casing of `PATH`).
  tasktree injects **`TASKTREE_ROOT`** (absolute resolved workspace root) into every step. No other
  built-in vars in v0 (per-source path vars may be added later).
- **Working directory:** the step's base CWD is the **resolved tasktree root**, then joined with
  `workdir` if set — **independent of where `tasktree apply` was invoked** (apply resolves the
  root from any subdirectory). `workdir` is validated **before** the step runs: it must resolve to
  an existing directory **within** the workspace root. Paths that escape the root (`..` chains,
  absolute paths outside root) are rejected with a validation error.
- **Output:** child stdout/stderr stream **live (passthrough)**, prefixed by a per-step header
  (e.g. `==> [install-api-deps] npm ci`). Headers and step output go to **stderr**, keeping stdout
  clean for tasktree's own (current/future) result output. Output is **not** buffered into the
  error (steps can emit megabytes).
- **Cancellation / timeout:** each step runs in its **own process group**. On Ctrl-C, context
  cancellation, or engine timeout, tasktree signals the whole group **SIGTERM → grace → SIGKILL**
  so child trees (`npm`/`node`) do not orphan and keep mutating the workspace. Unix-only for v0
  (Windows is not a core target).
- **Templating:** bootstrap fields (`name`, `run`, `workdir`, `env` values) participate in the
  existing `{{variable}}` rendering at **`init`** time, and are included in template
  parameter-completeness validation. Runtime `$VAR` shell expansion is orthogonal, handled by `sh`
  at apply time.

### Validation

- `name` — **required, non-empty, unique** within `spec.bootstrap` (used in headers/errors/logs).
- `run` — **required, non-empty.**
- `workdir` — optional; validated for existence + within-root at apply time (read-only existence
  *warning* under `--dry-run`).
- Enforced in **both** `tasktree.schema.json` (editor/CI) **and** a runtime check in the apply path
  (since `metadata.Store.Load` does not validate); the runtime check fails fast before any step
  runs.
- Omitted/empty `bootstrap` is a no-op (no error).

### CLI

```
tasktree apply [--skip-bootstrap] [--dry-run]
```

- **`--skip-bootstrap`** materializes sources only.
- **`--dry-run`** never executes steps; it prints the **ordered plan** (step name + `run` +
  `workdir`) plus any workdir-existence warnings. (No standalone `tasktree bootstrap` command, no
  single-step selection in v0 — `apply` with sources present is already the fast "re-run setup"
  path.)

---

## 2. Snapshot / Restore

A snapshot is a **lightweight, portable, self-contained capture of a workspace's concrete state**
that can be restored onto a fresh machine to reproduce the exact working tree. The implementation
plan lives in [snapshot-implementation-plan.md](./snapshot-implementation-plan.md).

### Artifact & format

The snapshot is a **single portable `.tar.gz`**. It is built in a temp staging dir and packed,
so the on-disk/transport surface is always one file. Internal layout:

```
snapshot.yaml            # manifest (yaml, consistent with Tasktree.yml)
Tasktree.yml             # embedded copy of the (rendered) spec → self-contained restore
bundles/<source>.bundle  # git: local commits base..HEAD (only if local commits exist)
dirty/<source>.tar       # git: working-tree content of dirty paths (only if dirty)
```

- **Self-contained:** the snapshot embeds `Tasktree.yml`, so restore needs nothing but the tarball
  (the whole point is reproduction on a *fresh* machine).
- The dirty payload is a **tar** (not zip): we are already inside a `.tar.gz`, and tar preserves
  unix file modes + symlinks (zip is lossy there).
- The inner `git bundle` and dirty tar are incompressible, so the outer gzip is a near-no-op on
  size; it is kept for a uniform single-file `.tar.gz` surface.

### Snapshotter structure

A snapshot is **workspace-level**, composed of one **sub-snapshot per source**. It is produced by
`internal/snapshot` **free functions** plus a `SnapshotService` **type-switch** in `internal/app`
— mirroring the *actual* materializer code (`materialize.Git` + `ApplyService.applySource`
switch), **not** a plugin interface or registry. Non-git source types produce **no sub-snapshot**
(inventory-only in the manifest) and are reproduced by re-applying the embedded spec on restore.

### Manifest (`snapshot.yaml`)

The manifest is **the place resolved SHAs are allowed to live**. `version: 1`; restore rejects an
unknown/newer major version.

```yaml
version: 1
createdAt: 2026-06-05T12:00:00Z
tasktree: feature-payments
sources:
  - name: api
    type: git
    git:
      remoteURL: git@github.com:myorg/api.git   # read from the on-disk origin
      branch: feature/payments
      detached: false
      baseSHA: <sha>          # always pinned for git sources
      headSHA: <sha>          # restore verifies HEAD matches this
      bundle: bundles/api.bundle   # omitted if no local commits
      dirty: dirty/api.tar         # omitted if clean
      includeIgnored: false
  - name: docs                # non-git: inventory only, reproduced from spec
    type: http
```

### Git `Snapshotter`

| Part | Encoding | Notes |
|---|---|---|
| Base | `merge-base HEAD <remote-ref>` (`remote-ref` = upstream → `origin/<branch>` → `origin/HEAD`); `baseSHA` + `remoteURL` in the manifest | re-materializable from the remote at that pin. **Always pinned, even when clean** (base==head) so restore reproduces the exact commit even if `origin/<branch>` moved. Rely-on-remote model; restore **verifies `HEAD==headSHA`** and fails loudly on mismatch. |
| Committed delta | **`git bundle`** of `base..HEAD` (HEAD-only in v0) | handles binaries + merge history in one file; fetched on restore. Chosen over `format-patch`, which mangles binaries/merges. Multi-branch capture is deferred. |
| Dirty state | one **tar** of the **working-tree content** of changed + untracked paths (`git status --porcelain -z --untracked-files=all`) | Captures content, not diffs (diffs mangle binaries). Includes a `dirty.manifest` member: deletions list + staged-path re-stage hint. **Partial-staging collapses to worktree content** (index-vs-worktree divergence on the same path → worktree wins; documented limitation). `.gitignore` respected by default (so `node_modules`/build dirs do not bloat); `--include-ignored` opts in. |

**Git edge cases:**

- **Detached HEAD:** record `detached: true`, empty `branch`, `headSHA`; base = `merge-base HEAD
  origin/HEAD`. Restore checks out detached at `headSHA`.
- **Local-only / never-pushed branch:** no upstream ⇒ base falls back to `merge-base HEAD
  origin/HEAD`; the bundle carries everything from that fork point.
- **No `origin` remote on disk:** **hard-fail at snapshot time** — the rely-on-remote model cannot
  reconstruct base without it. A self-contained-bundle fallback is a future enhancement.
- **Submodules — out of scope v0:** the gitlink pointer is part of the captured tree, but the
  submodule's own working tree/dirty state is not captured or restored.

### Other source types

Non-git source types produce **no sub-snapshot** and are reproduced from the embedded spec on
restore; they appear in the manifest as inventory-only entries (name + type) so restore can detect
a spec/snapshot mismatch:

- `http`, `archive`, `static` — reproducible from the spec; no-op.
- `local` — points outside the workspace boundary; no-op.

### Restore

Restore is **split by source type**, not a uniform "apply pinned" (apply has no pin capability):

- **Non-git sources:** plain `materialize` from the embedded spec.
- **Git sources** (each carries a pin): a dedicated deterministic path —
  1. clone via the existing **`cache.Manager`** (cache reuse, like apply); `origin` = `remoteURL`.
  2. ensure `baseSHA` is present (fetch from remote if the cache lacks it).
  3. if a bundle exists, `git fetch <bundle>` to bring `base..HEAD` objects in.
  4. recreate branch identity: branch `<branch>` at `headSHA`, or **detached at `headSHA`**.
  5. **verify `HEAD == headSHA`**; fail loudly on mismatch.
  6. unpack `dirty/<source>.tar` over the working tree, apply the deletions list, `git add` the
     staged-hint paths.

The materialize + git-restore + dirty-unpack phase restores into a **temp staging dir on the same
filesystem**, then `os.Rename` to the final target on full success (all-or-nothing). **Bootstrap
then runs on the final dir** (default; `--skip-bootstrap` opts out), **fail-fast / no rollback** —
identical to the apply contract — because bootstrap outputs (`node_modules`, generated config) are
normally `.gitignore`d and thus excluded from the snapshot, so a usable restore needs setup. On
success, restore **registers** the workspace in the global registry (like `init`).

This restore path is the same one the engine uses at startup, so it is exercised by both local use
and [tasktree-engine](./tasktree-engine.md).

### CLI

```
tasktree snapshot [--output <path>] [--include-ignored]
tasktree restore <snapshot> [--into <dir>] [--skip-bootstrap]
```

- **`tasktree snapshot`** resolves the workspace root from any subdirectory (like `apply`).
  Default output `./<metadata.name>-<UTC-timestamp>.tar.gz` (timestamp avoids clobber);
  `--output/-o <path>` overrides, and **`-o -` streams the tar.gz to stdout** (the engine's
  "return the canonical artifact" path). Output is written atomically (temp + rename). It
  **hard-fails if any declared source is not materialized** (no silent partial snapshot); a fully
  clean workspace yields a valid snapshot (pins only). No `--dry-run` in v0.
- **`tasktree restore`** accepts a tarball path or **`-` to read the tar.gz from stdin**. Default
  target `./<metadata.name>` from the embedded spec; **refuses a non-empty target** (no `--force`
  in v0).

---

## Relationship to tasktree-engine

These primitives are owned by tasktree core and have no dependency on the engine. The engine
consumes them: it runs `apply` (with bootstrap) inside a sandbox to materialize a task, and it
uses `snapshot` to produce the single canonical result artifact returned to the client. See
[tasktree-engine](./tasktree-engine.md).
