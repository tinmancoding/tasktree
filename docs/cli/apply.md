# tasktree apply

Materialize sources declared in `Tasktree.yml` that are not yet on disk, then run the workspace bootstrap steps.

## Synopsis

```
tasktree apply [flags]
```

## Description

Reads `Tasktree.yml` from the resolved tasktree root and ensures each declared source is present on disk. Sources whose destination path already exists are **skipped without error**, making source materialization safe to run repeatedly (idempotent).

The primary use case is reproducing a workspace from a committed `Tasktree.yml` — a teammate or CI runner runs `apply` after obtaining the spec.

All five source types (`git`, `http`, `archive`, `static`, `local`) are materialized. See [Source Types](../concepts/source-types.md).

After **all** sources are materialized, the steps in `spec.bootstrap` run sequentially as an unconditional phase — on **every** `apply`, regardless of whether any source was (re)materialized. See [Bootstrap](#bootstrap) below.

## Flags

| Flag | Description |
|---|---|
| `--dry-run` | Preview what would be done without cloning, modifying anything, or running bootstrap. Prints the ordered bootstrap plan and warns about missing workdirs. |
| `--skip-bootstrap` | Materialize sources only; do not run bootstrap steps. |

## Examples

Materialize all missing sources and run bootstrap:

```bash
tasktree apply
```

```
Using existing remote branch "feature/payments" from origin.
Cloned api at api
Skipped web (already present)
==> [install-api-deps] npm ci
...
```

Preview without making changes:

```bash
tasktree apply --dry-run
```

```
Would apply api at api (branch: feature/payments)
Skipped web (already present)
Bootstrap plan:
  1. install-api-deps: npm ci (workdir: api)
  2. generate-config: ./scripts/gen-local-config.sh (workdir: api)
```

Materialize sources only:

```bash
tasktree apply --skip-bootstrap
```

When all sources are already present:

```bash
tasktree apply
# All sources are already present.
# (bootstrap still runs unless --skip-bootstrap is given)
```

## Branch resolution on apply

`apply` uses the same branch resolution logic as `add`:

1. `git.branch` exists locally → check it out
2. `git.branch` exists on `origin` → create a local tracking branch
3. `git.branch` does not exist → create from `git.ref` (or the remote default branch)
4. No `git.branch` set → check out `git.ref` or the default branch directly

## Bootstrap

`spec.bootstrap` is an optional, ordered list of **idempotent** setup steps that run **after all sources are materialized**, on **every** `apply`. Use it for `npm ci`, `go mod download`, generating local config, and similar environment-convergence tasks.

```yaml
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
      workdir: api
      env:
        ENVIRONMENT: local
```

| Field | Required | Description |
|---|---|---|
| `name` | yes | Identifier for the step (used in logs/errors). Must be unique within `spec.bootstrap`. |
| `run` | yes | Shell command, executed via `sh -c`. |
| `workdir` | no | Working directory relative to the workspace root. Must stay within the workspace. Defaults to the root. |
| `env` | no | Extra environment variables for this step. Overrides inherited values on collision. |

### Execution model

- **Unconditional phase** — bootstrap runs on every `apply`. There is no completion marker and no hidden state; the honest model is that setup re-runs each time, so steps **must** be idempotent.
- **Sequential, fail-fast** — steps run in declared order. A non-zero exit aborts `apply` with an error naming the step, its `workdir`, and the child exit code (preserved, not flattened). Later steps do not run and **no rollback** is performed — fix the cause and re-run.
- **Shell** — each step runs as `sh -c "<run>"` (POSIX `sh`, not `bash`). Steps needing `bash` must invoke it explicitly.
- **Environment** — the child inherits tasktree's environment, with the step's `env` overlaid on top (step `env` wins on collision). tasktree injects `TASKTREE_ROOT` (the absolute, resolved workspace root) into every step.
- **Working directory** — the base CWD is the resolved workspace root, then joined with `workdir`. This is independent of where you invoke `tasktree apply`. A `workdir` that does not exist or escapes the workspace root is rejected before the step runs.
- **Output** — per-step headers (`==> [name] run`) and live child stdout/stderr stream to **stderr**, keeping stdout clean.
- **Cancellation** — each step runs in its own process group; Ctrl-C or an engine timeout sends `SIGTERM → grace → SIGKILL` to the whole group so child processes (`npm`, `node`, …) do not orphan. Unix only.
- **Templating** — bootstrap fields participate in `{{variable}}` rendering at `init` time and are included in template parameter-completeness validation. Runtime `$VAR` expansion is handled by `sh` at apply time.

!!! note "Bootstrap is environment convergence"
    Bootstrap is conceptually part of "materializing the desired state," like cloning a repo — so it lives in `Tasktree.yml`. If a step is not idempotent/reproducible, it does not belong in bootstrap.

## Notes

- `apply` does not update existing checkouts. It only materializes sources that are missing. To update an existing checkout, use `git fetch` / `git pull` inside the checkout directory.
- To capture and reproduce the **concrete** working state (exact commits + uncommitted edits) rather than the desired state, use [`tasktree snapshot`](snapshot.md) and [`tasktree restore`](restore.md).
