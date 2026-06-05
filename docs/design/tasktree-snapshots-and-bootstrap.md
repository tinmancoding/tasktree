# Design: Bootstrap & Snapshot/Restore

Status: **Bootstrap — Accepted** · **Snapshot/Restore — Proposed**
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

A snapshot is a **lightweight, portable capture of a workspace's concrete state** that can be
restored onto a fresh machine to reproduce the exact working tree.

### Data model

A snapshot is **workspace-level**, composed of one **sub-snapshot per source**. It is produced by
a **pluggable, per-source-type `Snapshotter`** interface (mirroring the existing per-source-type
materializer pattern). The **default implementation is a no-op** — such sources are reproduced
purely by re-running `apply` from the spec.

```
snapshot/
├── snapshot.yaml        # manifest: per-source base pins + which sources have sub-snapshots
├── bundles/
│   └── api.bundle       # git: local commits beyond base (git bundle)
└── dirty/
    └── api.zip          # git: staged + unstaged + untracked (gitignore-respected)
```

#### Manifest (`snapshot.yaml`)

The manifest is **the place resolved SHAs are allowed to live**. For each source it records the
concrete state needed to reconstruct it:

- the resolved **base SHA** and remote URL (git sources),
- which artifacts (bundle, dirty archive) are present.

#### Git `Snapshotter`

| Part | Encoding | Notes |
|---|---|---|
| Base | resolved base SHA + remote URL in the manifest | re-materializable from spec at that pin |
| Committed delta | **`git bundle`** of local commits beyond base | handles binaries, multiple branches, merge history in one file; can be fetched from on restore. Chosen over `format-patch`, which mangles binaries/merges. |
| Dirty state | one **zip** of staged + unstaged + untracked | **`.gitignore` respected by default** (so `node_modules`/build dirs do not bloat the snapshot). `--include-ignored` opts into capturing ignored files. |

#### Other source types

Each source type may provide its own `Snapshotter`. It is acceptable to leave these as **no-op**
and rely on the `apply` stage for reproduction:

- `http`, `archive`, `static` — reproducible from the spec; no-op.
- `local` — points outside the workspace boundary; no-op by default.

### Restore

Restore reproduces the exact working state, on a fresh machine or in place:

1. Re-apply the spec **pinned to the recorded base SHAs**.
2. `git fetch` / replay the **bundle** to restore local commits.
3. Unpack the **dirty archive** to restore staged/unstaged/untracked edits.

This restore path is identical to the engine's startup path, so it is exercised by both
local use and [tasktree-engine](./tasktree-engine.md).

### CLI (proposed)

```
tasktree snapshot [--output <path>] [--include-ignored]
tasktree restore <snapshot> [--into <dir>]
```

---

## Relationship to tasktree-engine

These primitives are owned by tasktree core and have no dependency on the engine. The engine
consumes them: it runs `apply` (with bootstrap) inside a sandbox to materialize a task, and it
uses `snapshot` to produce the single canonical result artifact returned to the client. See
[tasktree-engine](./tasktree-engine.md).
