# Design: tasktree-engine

Status: **Proposed**
Layer: **tasktree-engine** (depends on tasktree core; lives in this repository)

tasktree-engine is an **AI sandboxing + delegation** layer built on tasktree. It takes a tasktree
plus a task definition and **runs that task somewhere** — locally in a container today, on AWS ECS
soon — in an isolated, materialized workspace, then returns a single canonical snapshot of the
result.

## Three-tier architecture

| Tier | Name | Owns |
|---|---|---|
| 1 | **`tasktree`** (core) | sources/apply, `spec.bootstrap`, snapshot/restore. Lean (git + fs only). See [bootstrap & snapshot/restore](./tasktree-snapshots-and-bootstrap.md). |
| 2 | **`tasktree-engine`** | `kind: Task`, engine driver interface, run lifecycle/registry, transport, result views. **This document.** |
| 3 | **`pi-transport`** *(deferred)* | agentic orchestration / fan-out, likely a pi extension, atop tasktree-engine. |

tasktree-engine ships as its own binary in this repository, depending on tasktree as a library so
it can run `apply` + `bootstrap` + `snapshot` under the hood. Heavy dependencies (Docker SDK, AWS
SDK) stay out of the lean core.

## Core principle: the engine is a dumb executor

The engine **runs an arbitrary command in an isolated, materialized tasktree**. It learns nothing
about agent loops, turns, tool calls, or "doneness." Running a shell script and running an agent
session (`pi`, `claude`, `opencode`) are *the same thing* — a command. This invariant keeps the
engine simple and uniform across drivers, and pushes all agentic concerns up to tier 3.

---

## The `Task` resource

A new `kind: Task` (same `tasktree.dev/v1` family). It wraps a tasktree with an execution and an
output definition, while the underlying `kind: Tasktree` stays pure desired state.

```yaml
apiVersion: tasktree.dev/v1
kind: Task
metadata:
  name: fix-BUG-123
spec:
  tasktree:                     # inline, or $ref: ./Tasktree.yml
    sources:
      - name: api
        type: git
        git:
          url: git@github.com:myorg/api.git
          branch: fix/BUG-123
    bootstrap:
      - name: deps
        run: npm ci
        workdir: api
  run:
    image: ghcr.io/tinmancoding/tasktree-runtime:polyglot
    steps:
      - pi run --prompt "Fix the failing test in api/"
    env:
      MODEL: claude-sonnet
    resources:
      cpu: "2"
      memory: 4Gi
      timeout: 30m
  outputs:
    mode: snapshot              # canonical artifact; views derived client-side
```

- **`spec.tasktree`** — inline tasktree spec or a `$ref` to a `Tasktree.yml`.
- **`spec.run`** = `{ image, steps[], env?, resources? }`:
  - **`image`** is pinned in the Task (a "what I need"); the agent CLI is **baked into the image**,
    not injected by the engine. Prebuilt base images are shipped as defaults but any image works.
    For ECS the image must be pullable (ECR).
  - **`steps`** run **fail-fast**, sequentially.
  - **`resources`** (cpu/memory/timeout) are hints the engine maps to `docker run` limits / ECS
    task sizing. `timeout` is the hard backstop.
- **`spec.outputs.mode`** — the engine always produces one canonical **snapshot**; views are
  derived client-side.

The **engine is not part of the Task.** You define *what* in the Task and choose *where* at
invocation time ("run this task somewhere"). Engine selection is a runtime parameter.

---

## Execution model

### Ship the spec; engine materializes inside the sandbox

The input artifact is the **spec** (the `Task` + tasktree spec — a few KB), not pre-materialized
sources. Inside the sandbox the engine runs:

```
apply (clone sources)  →  bootstrap  →  run.steps  →  snapshot
```

This keeps transport tiny and uniform across drivers, and reuses the core `apply`/`bootstrap`/
`snapshot` paths. The cost — the sandbox needs credentials to clone — is handled below.

### Lifecycle: async-first

Submitting a task returns a **run** (the unit of identity) with states:

```
pending → preparing (apply+bootstrap) → running → snapshotting → succeeded | failed | cancelled
```

- Clients **stream logs**, **poll status**, and **fetch the result** against a run ID — identical
  for local and remote.
- **`--wait`** is foreground sugar: submit, block-and-stream until terminal, auto-fetch.
- **Run-state authority:** local = a runs registry under `~/.local/state/tasktree-engine/...`
  (mirrors tasktree's registry); remote = **S3 is the source of truth** (a run ID maps to an S3
  prefix plus the ECS task ARN; the client keeps a lightweight pointer).
- **Three transport artifacts:** **spec in / logs / snapshot out.**

### Done & exit semantics

- **Success = exit code.** Steps fail-fast; the run `succeeded` only if all steps exit 0.
- **An agent owns its own "done"** and signals it by exiting. The engine never interprets it.
- **`timeout`** kills the process → status `timeout` (a flavor of failed).
- **Always snapshot on any terminal state** (`succeeded`, `failed`, `timeout`, `cancelled`) —
  best-effort; partial work is the most valuable thing to inspect on failure.
- Engine-`succeeded` means **"ran cleanly," not "task accomplished."** Real judgment happens at
  human diff review.

---

## Engine drivers

A driver interface — `Prepare → Bootstrap → Run → Snapshot → Fetch → Teardown` — with the engine
selected at invocation:

| Driver | Status | Compute | Transport | Cache |
|---|---|---|---|---|
| **local-oci** (Docker/Podman) | v0 | local container | local volume / bind-mount | mount host bare cache |
| **ecs-s3** (AWS) | fast-follow | ECS task | S3 (spec/logs/snapshot objects) | cold clones in v0; EFS/S3-sync later |

- **Resources** map to `docker run` limits / ECS task sizing.
- **Concurrency:** each run is fully independent (own container/task, own fresh ephemeral
  workspace, own RO cache mount). N concurrent runs supported natively, bounded by a configurable
  local max / ECS account limits. **No built-in fan-out** (one Task → many variants) — that is
  pi-transport's job.
- **Retention:** local artifacts persist under `~/.local/state/tasktree-engine/runs/<id>/`;
  container auto-removed on terminal (`--rm`) — **logs + snapshot survive and are sufficient for
  post-mortem** (a `--keep` flag can preserve the container). Remote: S3 lifecycle policy expires
  objects after a TTL; `tasktree-engine prune --older-than <dur>` and `tasktree-engine rm <run>`
  for hygiene (mirroring `tasktree prune`).

---

## Security model

Sandboxing protects two distinct things; the engine addresses both, with explicit, documented v0
limits.

### Host isolation
The container/microVM fences the host filesystem, processes, and network. ✅

### Credentials & blast radius

- **Read-only / least-privilege by default. No push from the sandbox.** With the ship-the-spec +
  snapshot model the sandbox only ever needs **read/clone** access. Results return as the snapshot;
  **the client reviews the diff and pushes with real credentials** after review. Write access never
  enters the sandbox.
- **Credentials are engine-level config** (a dedicated GitHub PAT *or* an SSH key), provisioned
  once and **reused across all tasks** — never carried in the `Task` spec. Because the engine runs
  `tasktree` commands under the hood, those credentials must already exist in the engine
  environment as a precondition.
  - local-oci: forward the **SSH agent socket** (ephemeral, no key material copied in), or supply
    a read-only token.
  - ecs-s3: a **short-lived, repo-scoped, read-only token** in Secrets Manager, injected at task
    start with a short TTL.
- **AI API keys** are injected as run-scoped env, ideally behind a proxy/gateway with spend + rate
  limits so a leaked key has bounded blast radius.
- **Write/elevated profile** (deploy, publish) is a **deferred, explicit opt-in** — not the
  default.

### Network egress
Engine-level policy knob, **capped server-side** (a Task may request a level but cannot widen it):

| Mode | Use |
|---|---|
| `open` | **v0 local default.** Frictionless bootstrap + agent API calls. |
| `allowlist` | ECS / untrusted / shared: proxy permitting only the AI endpoint(s), git host, package registries. |
| `none` | fully offline, everything vendored. |

> **v0 local is FS-isolation only — not exfil-proof.** Even `allowlist` is defense-in-depth (data
> can be smuggled through the allowed AI endpoint). The real secret protection is *no write
> credentials in the box* + a bounded/proxied API key.

### Cache poisoning
The bare cache is mounted **read-only** into the sandbox by default; the **trusted engine host**
refreshes it outside the sandbox. A read-write mount would let an untrusted agent poison the shared
cache for all future runs — acceptable only while it is your own trusted agent on local-oci.

---

## Result extraction

The engine emits **exactly one canonical artifact — the snapshot**
(see [snapshot/restore](./tasktree-snapshots-and-bootstrap.md)). Every view is a **client-side
derivation**, no re-run:

| Command | Result |
|---|---|
| `fetch --zip [--source <name>]` | reconstructed working tree(s), zipped — the "full tasktree" view |
| `fetch --diff [--source <name>] [--no-dirty]` | unified patch(es) for review/PR |
| `restore` / `apply-result [--into <dir>]` | lay the snapshot onto a local tasktree (fresh or existing) to build/test/push |

- **Diffs are computed against the recorded base SHA** (what the run actually changed), not live
  upstream HEAD. Deterministic and honest.
- **Upstream reconciliation** (a moved base) is a merge concern handled at `restore`/push time with
  real credentials — never baked into the diff.

---

## CLI sketch

```
tasktree-engine run <task.yaml> [--engine local-oci|ecs-s3] [--wait]
tasktree-engine status <run>
tasktree-engine logs <run> [--follow]
tasktree-engine fetch <run> [--zip|--diff] [--source <name>] [--no-dirty]
tasktree-engine restore <run> [--into <dir>]
tasktree-engine ls
tasktree-engine rm <run>
tasktree-engine prune [--older-than <dur>]
```

---

## Deferred / out of scope

- **pi-transport** agentic layer and **fan-out / best-of-N** orchestration (tier 3).
- **Write/elevated credential profile** for deploy/publish tasks.
- **ECS cache optimization** (EFS / S3-sync).
- **API-key proxy/gateway** with spend limits.

## Loose ends to finalize

1. **`kind: Task` schema** — `outputs` defaults, `run.resources` fields, inline vs `$ref` for the
   tasktree.
2. **Version contract** between `tasktree` and `tasktree-engine` (engine requires a core version
   exposing snapshot/bootstrap).
3. **Snapshot manifest format** — concrete `snapshot.yaml` + `bundles/` + `dirty/` layout
   (shared with the core design doc).
