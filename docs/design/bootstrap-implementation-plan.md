# Implementation Plan: Bootstrap

Status: **Implemented** · Tracks the **Bootstrap — Accepted** section of
[tasktree-snapshots-and-bootstrap.md](./tasktree-snapshots-and-bootstrap.md).

This plan delivers `spec.bootstrap`: an ordered list of idempotent setup steps run as an
unconditional phase after sources materialize on every `apply`. Snapshot/restore is **out of
scope** (separate increment).

## Status: all phases delivered

Phases 1–6 are complete and merged into the working tree. Code: `internal/domain/bootstrap.go`,
`internal/bootstrap/` (executor + unix/other procgroup), `internal/app/apply_service.go`,
`internal/cli/apply.go`, `internal/app/init_service.go`, `schema/*.json`,
`examples/templates/fullstack-feature.yml`, `README.md`. All tests + `go vet` + `gofmt` pass;
end-to-end smoke tested (success, fail-fast exit-code propagation, `--skip-bootstrap`, `--dry-run`
plan, subdir invocation, `TASKTREE_ROOT` injection, workdir resolution).

## Decisions locked (from design interview)

| # | Decision |
|---|---|
| Phase model | Bootstrap is an **unconditional phase** after source materialization; runs on every `apply` regardless of source skip status. No completion marker, no hidden state. |
| Source idempotency | Improving source-level apply refresh is **out of scope** (tracked separately). |
| Shell | `sh -c "<run>"` (POSIX sh, not bash). |
| Env | Inherit tasktree env + per-step `env` overlay (**step wins**). Inject `TASKTREE_ROOT` only. |
| CWD | Base CWD = resolved tasktree root, then `+ workdir`. Independent of invocation CWD. |
| workdir | Pre-flight existence check + **within-root** restriction (reject `..`/absolute escapes). |
| Output | Live passthrough, per-step header, **to stderr**. Not buffered into errors. |
| Failure | Fail-fast, **no rollback**, preserve child **exit code**, named-step error, CLI exits non-zero. |
| Cancellation | Own **process group**, SIGTERM → grace → SIGKILL on cancel/timeout. **Unix-only** v0. |
| Templating | Bootstrap fields rendered at `init`; included in parameter-completeness validation. |
| Validation | `name` required/non-empty/**unique**; `run` required/non-empty. Enforced in **schema + runtime**. |
| Dry-run | Never executes; prints **ordered plan** + read-only workdir-existence warnings. |
| CLI surface | `tasktree apply [--skip-bootstrap] [--dry-run]` only. No standalone `bootstrap` command. |

---

## Phase 1 — Domain model & validation

**Files:** `internal/domain/tasktree.go`, `internal/domain/errors.go`, `internal/domain/bootstrap.go` (new), tests.

1. Extend `WorkspaceSpec`:
   ```go
   type WorkspaceSpec struct {
       Sources   []SourceSpec    `yaml:"sources"`
       Bootstrap []BootstrapStep `yaml:"bootstrap,omitempty"`
   }
   ```
2. Add the step type:
   ```go
   type BootstrapStep struct {
       Name    string            `yaml:"name"`
       Run     string            `yaml:"run"`
       Workdir string            `yaml:"workdir,omitempty"`
       Env     map[string]string `yaml:"env,omitempty"`
   }
   ```
3. Add `ValidateBootstrap(steps []BootstrapStep) error` (in `bootstrap.go`):
   - `name` non-empty; `run` non-empty; `name` unique across the list.
   - Returns typed errors (new entries in `errors.go`, mirroring `InvalidAnnotationKeyError`):
     `EmptyBootstrapFieldError{Index, Field}`, `DuplicateBootstrapNameError{Name}`.
   - **Does not** resolve/validate `workdir` here (that needs the on-disk root; done at apply time).

**Tests:** table-driven validation cases (missing name/run, duplicate names, valid, empty list).

---

## Phase 2 — Bootstrap executor package

**Files:** `internal/bootstrap/executor.go` (new), `internal/bootstrap/executor_unix.go` (new, process-group), tests.

Mirror the `materialize` package style (pure, dependency-injected, no CLI/IO coupling beyond
provided writers).

1. **Plan resolution** — `ResolveStep(root string, step domain.BootstrapStep) (resolved, error)`:
   - Join `root` + `workdir`, `filepath.Clean`, verify the result stays within `root`
     (reject escapes → `WorkdirEscapesRootError`).
   - Stat the dir: must exist + be a directory (→ `WorkdirNotFoundError`). Used by both the
     executor (hard error) and dry-run (warning).
2. **Execution** — `Run(ctx, root string, steps []domain.BootstrapStep, opts Options) error`:
   - `opts` carries `Stderr io.Writer` (header + child stdout/stderr target) and base env source.
   - For each step in order:
     - Print header `==> [<name>] <run>` to `opts.Stderr`.
     - Build `exec.CommandContext(ctx, "sh", "-c", step.Run)`.
     - `cmd.Dir = resolvedWorkdir`.
     - `cmd.Env = mergeEnv(os.Environ(), {"TASKTREE_ROOT": root}, step.Env)` (step wins).
     - `cmd.Stdout = cmd.Stderr = opts.Stderr` (live passthrough, no buffering).
     - Set `cmd.SysProcAttr` for a new process group (Unix file).
     - On `ctx` cancel/timeout: send SIGTERM to the group, wait a short grace, then SIGKILL.
     - On non-zero exit: return `StepFailedError{Name, Workdir, ExitCode}` (extract code from
       `*exec.ExitError`); **fail-fast**, no further steps, no rollback.
3. **Process-group control** isolated in `executor_unix.go` (`//go:build unix`) so the rest stays
   portable; a `_other.go` stub can fall back to plain `CommandContext` if ever needed.

**Tests:** use real `sh` (skip if absent). Cover: success ordering, env overlay + `TASKTREE_ROOT`
visible, `workdir` honored, non-zero exit → `StepFailedError` with correct code + later steps not
run, context cancel kills a `sleep` child promptly (no orphan), live output reaches the writer.

---

## Phase 3 — ApplyService integration

**Files:** `internal/app/apply_service.go`, tests.

1. Extend `ApplyOptions`:
   ```go
   type ApplyOptions struct {
       DryRun        bool
       SkipBootstrap bool
   }
   ```
2. Add a `bootstrap` collaborator to `ApplyService` (inject the executor; keep it an interface for
   tests, like `gitx.Client`/`cache.Manager`).
3. In `Run`, after the source loop succeeds:
   - If `SkipBootstrap` or `len(spec.Spec.Bootstrap)==0` → skip.
   - **Runtime validation** first: `domain.ValidateBootstrap(...)` → fail before executing.
   - If `DryRun`: build a plan (`[]BootstrapPlanStep{Name, Run, Workdir, WorkdirExists}`) using
     `ResolveStep` (existence only, no execution) and attach to `ApplyResult`. Do **not** run.
   - Else: call `bootstrap.Run(ctx, root, steps, Options{Stderr: ...})`. Propagate the error
     (wrapped to name the phase) so apply aborts non-zero.
4. Extend `ApplyResult` with `BootstrapPlan []BootstrapPlanStep` (dry-run) and/or
   `BootstrapRan bool` for CLI reporting.

**Tests:** fake bootstrap collaborator; assert it's invoked after sources, skipped on
`--skip-bootstrap`, not invoked when list empty, plan populated on dry-run, error from executor
aborts `Run`.

---

## Phase 4 — CLI wiring

**Files:** `internal/cli/apply.go`, `internal/cli/root.go` (deps), tests.

1. Add `--skip-bootstrap` bool flag.
2. Wire `app.ApplyOptions{DryRun, SkipBootstrap}`.
3. Pass `cmd.ErrOrStderr()` as the executor's `Stderr` writer (headers/output to stderr).
4. Dry-run output: after the existing per-source "Would apply…" block, print the bootstrap plan
   to stdout (`Bootstrap plan:` + numbered `name` / `run` / `workdir`), and emit
   `warning: workdir %q does not exist` lines for missing dirs.
5. Non-dry-run: optionally print a one-line `Running bootstrap (<n> steps)…` before the executor
   streams. Ensure `RunE` returns the executor error via `formatError` so exit code is non-zero.
6. Wire the executor into `defaultDependencies()` and `NewApplyService(...)`.

**Tests:** CLI golden/behavior tests for flag plumbing, dry-run plan rendering, skip path.

---

## Phase 5 — Template rendering & parameter validation

**Files:** `internal/app/init_service.go`, tests.

1. In `renderTemplate`, after rendering sources, render bootstrap:
   ```go
   for _, b := range t.Spec.Bootstrap {
       rendered := domain.BootstrapStep{
           Name:    variable.RenderString(b.Name, values),
           Run:     variable.RenderString(b.Run, values),
           Workdir: variable.RenderString(b.Workdir, values),
           Env:     variable.RenderStringMap(b.Env, values),
       }
       bootstrap = append(bootstrap, rendered)
   }
   ```
   and set `Spec.Bootstrap` on the returned `TasktreeSpec`.
2. In `extractAllRefs`, append bootstrap field strings (`Name`, `Run`, `Workdir`, env keys+values)
   so unresolved `{{vars}}` in bootstrap are caught by parameter-completeness validation.

**Tests:** template with `{{var}}` in bootstrap renders correctly; missing required param in a
bootstrap field is reported.

---

## Phase 6 — JSON schema

**Files:** `schema/tasktree.schema.json`, an example template, docs.

1. Add `bootstrap` to `spec.properties`:
   - `type: array`, items = object with `additionalProperties:false`,
     `required: ["name","run"]`,
     `name` (pattern, non-empty), `run` (non-empty string), `workdir` (string),
     `env` (object, `additionalProperties: {type:string}`).
2. Add a bootstrap example to one of `examples/templates/*.yml` (e.g. `fullstack-feature.yml`).
3. Update `README.md` bootstrap section + `mkdocs.yml` if a new doc page is warranted.

**Note:** schema name-uniqueness isn't expressible in JSON Schema → that invariant is the runtime
check's job (Phase 1/3); the schema covers shape + required + non-empty.

---

## Sequencing & verification

Order: **1 → 2 → 3 → 4** (functional vertical slice), then **5 → 6** (templates/schema polish).

Each phase: `make build` + `go test ./...`. Final acceptance:
- `tasktree apply` runs bootstrap after sources, streams to stderr, fails fast with exit code.
- `tasktree apply --skip-bootstrap` materializes only.
- `tasktree apply --dry-run` lists the plan + workdir warnings, executes nothing.
- `tasktree init` from a template with `{{vars}}` in bootstrap renders + validates them.
- Ctrl-C during a long step kills the child tree (no orphans).

## Open/deferred (not in this increment)

- Opt-in bootstrap result cache / completion markers.
- Per-source path env vars (`TASKTREE_SOURCE_<name>`).
- Standalone `tasktree bootstrap` / single-step selection.
- Windows process-group handling.
- Source-level apply idempotency/refresh.
