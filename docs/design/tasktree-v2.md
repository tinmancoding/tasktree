# Design: tasktree V2

Status: **Accepted** (design interview, decisions D1–D59)
Supersedes: the workspace-centric V1 model (`Tasktree.yml` + plain folders + registry)

V2 turns tasktree into a single personal tool for **task management + multi-repo workspace
management**, using git for all state:

1. A hidden, central **tasks repository** holds task records on `main` and one orphan branch
   per tasktree.
2. A tasktree's repositories are **git submodules** (full clones), so a tasktree's state is
   versioned by ordinary commits.
3. The task record is a **living markdown document** — the personal equivalent of a Jira
   ticket — so no external task system is required.

Decision codes (`Dn`, `Kn`, `Xn`, `Rn`, …) refer to the [decision log](#decision-log).

---

## Goals and non-goals

**Goals**

- One tool for tasks and their workspaces; git is the only state store.
- Version everything a task produces: root artifacts (`AGENTS.md`, research, drafts) and the
  exact commits of every repo (gitlinks).
- Safe concurrent use by a human and several agents on one machine; eventual multi-machine use.
- Agents and humans use the CLI, never git in the tasktree root.

**Non-goals (V2.0)**

- Team-shared task boards (single-person system, D1).
- Replacing or syncing with Jira/GitHub (references only, D25).
- Non-git source types (D11).
- Snapshot/restore and the tasktree-engine (K6, K7).

## Principles

- **P1 — Submodules are an implementation detail.** Nobody runs git in a tasktree root; git
  inside a submodule works as usual. Enforced by hiding the root's git dir (D15).
- **One owner per fact.** Task facts live only on `main` (D7); per-machine facts live only in
  `local.yml` (D17); repo facts live only in `.gitmodules` + gitlinks (D11).
- **The tool commits what the tool changed; you commit what you changed** (D13′, D14).
- **Never move work silently.** Dirty or non-fast-forward repos are skipped and reported,
  never forced or rebased (D14).

---

## Architecture

### Filesystem layout (D21)

```text
~/.tasktree/                      # TASKTREE_HOME (overridable)
  config.yml                      # machine config (D19)
  local.yml                       # per-machine checkout state + focus (D17, D30)
  git/                            # hub: non-bare, main checked out (D9)
    tasks/<id>.md                 # task records (D6)
  clones/<id>.git/                # hidden git dir of each open task clone (D15)
    modules/<name>/               # submodule git dirs
  templates/                      # user templates (K5)
  aliases.yml                     # repo aliases (K1)

~/Library/Caches/tasktree/repos/<sha256-of-url>   # bare clone cache, unchanged from V1 (L2)

~/Developer/tasks/<handle>/       # tasktree working dirs, flat (L1), no .git at root
  .tasktree/config.yml            # committed marker + config (D12)
  .tasktree/task.md               # gitignored, generated record view (AG4)
  .gitmodules
  AGENTS.md, notes, research/ …   # committed artifacts
  api/  web/ …                    # submodules (full clones)
```

Everything under `~/.tasktree` is non-regenerable state and must be backed up (R4). The cache
is the only regenerable data and stays in the OS cache directory.

### Git topology (D2, D3, D9, D10, D15)

```text
              (optional) tasks remote            D19/D20 — none configured initially
                        ▲  sync (M5)
                        │
      ~/.tasktree/git  (hub, non-bare, main checked out)
        ├─ main            task records — written by the CLI under flock
        ├─ task/<id-a>     orphan branch
        └─ task/<id-b>     orphan branch
            ▲ push / fetch
            │
      task clone  (git dir: ~/.tasktree/clones/<id>.git,
                   work tree: ~/Developer/tasks/<handle>)
        └─ submodules cloned from the bare cache (no alternates)
```

- **No `git worktree` anywhere** (D2). Each open task is a full clone of the hub, checked out
  on `task/<id>` (D3). Git refuses pushes only to the hub's checked-out branch (`main`), so
  task branches push freely.
- **Task branches are orphans** (D10): they never contain `tasks/*.md`.
- **Hidden root git dir** (D15): the clone is created with `--separate-git-dir` and the root
  `.git` file is removed. The CLI runs root operations with
  `GIT_DIR=~/.tasktree/clones/<id>.git GIT_WORK_TREE=<root>`. `git status` in the root
  reports "not a git repository"; inside `api/`, git works normally.
- Submodule `.git` files and `core.worktree` use **absolute paths** (R1). Git writes relative
  paths (`gitdir: ../../clones/…`, `core.worktree ../../../../<dir>/api`, F8); the CLI rewrites
  them to absolute after `add`, `open`, and `import` (D40).
- **Never export `GIT_*`** (F7, D40). The CLI passes `GIT_DIR`/`GIT_WORK_TREE` per root git
  invocation only; exported, they leak into child processes (e.g. `git clone` of a submodule
  fails with "working tree already exists"). Inherited repository-context variables (the list
  from `git rev-parse --local-env-vars`, e.g. `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`) are
  stripped from the environment of child git processes, bootstrap steps, and any other spawned
  process. Auth and transport settings (`GIT_SSH_COMMAND`, `GIT_ASKPASS`, `GIT_CONFIG_*`) pass
  through.

### Writes and concurrency (D9)

- **Local (one machine):** every write to `main` takes an `flock` on a lock file in the hub,
  edits `tasks/<id>.md`, and commits with porcelain. Kernel-released locks mean no stale
  locks; writes take milliseconds, so concurrent agents simply queue.
- **Manual edits** in the hub are allowed; the CLI refuses to write while the hub has
  uncommitted manual changes and never sweeps them into its own commit.
- **Cross-machine (M5):** optimistic. `sync` fetches, fast-forwards or semantically merges
  (below), commits, and pushes with `--force-with-lease`; a rejected push retries the sync.

---

## Task model

### Identity (D5)

- `id`: immutable, random, 6-char Crockford base32 (e.g. `k7x2mq`). Used for branch
  `task/<id>`, clone dir, and `parent` references. Collision-free across offline machines.
- `handle`: mutable, human-facing — the Jira key if given, otherwise a slug of the title.
  Used for directory names and CLI arguments. Duplicate handles (e.g. after sync) are a soft
  conflict: the CLI disambiguates and `doctor` reports them.

### Statuses (D16′)

| Category | Statuses |
|---|---|
| Open | `incoming`, `next`, `active`, `blocked` |
| Closed | `done`, `dropped` |

`list` defaults to open tasks. A closed status on a checked-out task triggers the close flow.

### Record format (D6, D8)

One file per task: `tasks/<id>.md` on `main`. Three sections with distinct write rules:

| Section | Write rule | CLI |
|---|---|---|
| Frontmatter | typed fields, 3-way mergeable | `mark`, `ref`, `label`, `info set` |
| `## Description` | mutable, replaced whole | `info edit` |
| `## Log` | append-only, entries never edited | `note` |

```markdown
---
id: k7x2mq
handle: DSA-8946
title: AI board member prod admin
status: active
parent: null
labels: [aibm]
refs:
  - jira: DSA-8946
  - pr: DiligentCorp/boardroom-ai#123
  - url: https://diligentbrands.atlassian.net/wiki/spaces/X/pages/123
createdAt: 2026-10-12T09:00:00Z
updatedAt: 2026-10-12T10:03:00Z
---

## Description

What and why. Replaced as a whole by `tasktree info edit`.

## Log

### 2026-10-12T10:03:00Z note laci@mbp-work {#e-01jab3}
Retry budget = 3, backoff 200ms.

### 2026-10-12T11:20:00Z decision pi@mbp-work {#e-01jab9}
Use the existing client wrapper instead of a new SDK.
```

- Log entry kinds: `note`, `comment`, `decision`, `ref`. Each has an entry ID, timestamp,
  author, machine.

**Syntax rules (D38):**

- `## Description` and `## Log` are reserved H2 headings. `## Log` is always the last section.
- A log entry is a header line followed by its body, up to the next entry header:
  `### <RFC3339 UTC> <kind> <author>@<machine> {#e-<id>}`.
- Entry IDs use the same 6-char Crockford base32 scheme as task IDs (D5), unique per file.
- The parser is lossless: unknown frontmatter keys and hand edits made in the hub (D9) are
  preserved on rewrite.
- `info edit` rejects a description containing a reserved heading.
- **No stored changelog.** The CLI writes structured commit messages
  (`task(k7x2mq): status next → active`, `task(k7x2mq): note added`), so
  `git log -- tasks/<id>.md` is the changelog (`tasktree history`).
- **Refs are typed** (`jira`, `pr`, `url`); URLs are built from configured bases
  (`jira.baseUrl`). No external API calls (D25).
- Subtasks are ordinary records with `parent: <id>` (D4). No directory nesting.

### Semantic merge (M5, D8)

When `main` diverges across machines, merge per file:

- Log: union of entries by entry ID.
- Frontmatter: 3-way merge per field.
- Description: 3-way text merge; on a real conflict keep both versions and flag the task.

### Local state (D17, D30)

`~/.tasktree/local.yml` is never synced:

```yaml
checkouts:
  k7x2mq:
    path: /Users/landrasi/Developer/tasks/DSA-8946
    focus: p3n8wd        # optional focused subtask (D30)
```

---

## Tasktree model

### Branch contents (D11, D12)

A materialized tasktree branch contains only:

- submodules (`.gitmodules` + gitlinks),
- committed files,
- committed symlinks (stored as-is; absolute targets are machine-specific).

There is **no `Tasktree.yml`**. `.gitmodules` is the repo spec; `.tasktree/config.yml` holds
what git cannot express.

```ini
# .gitmodules
[submodule "api"]
    path = api
    url = git@github.com:DiligentCorp/api.git
    branch = feature/payments
    tasktree-mode = work
```

```yaml
# .tasktree/config.yml  (mandatory; root marker)
apiVersion: tasktree.dev/v2
kind: TasktreeConfig
taskId: k7x2mq
bootstrap:                  # optional (K4)
  - name: deps
    run: npm ci
    workdir: api
```

The gitlink records concrete state; `.gitmodules` `branch` records intent. Together they
replace V1's `Tasktree.yml` and snapshot base pin. Root discovery walks up to the nearest
`.tasktree/config.yml` declaring `kind: TasktreeConfig`; other files at that path (notably the
machine config `~/.tasktree/config.yml`) are skipped.

### Repo modes (D14)

Stored as `tasktree-mode` in `.gitmodules` (git ignores unknown keys). Default derived from
`add` flags (K2).

| Mode | Use | Update behaviour | Default when |
|---|---|---|---|
| `follow` | reference repo | fast-forward to `origin/<branch>` | `add` without `--branch` |
| `work` | your task branch | fast-forward only from `origin/<your-branch>`; reports upstream drift ("origin/main is 14 ahead of your base"); never rebases | `add --branch` |
| `pin` | fixed tag/SHA | never moves | `add --from <tag\|sha>` without `--branch` |

In every mode, a dirty submodule or non-fast-forward is skipped and reported. Change with
`tasktree mode <repo> <mode>`.

### Materialization

`add` resolves the alias, ensures the bare cache (`fetch --all --prune`), clones the submodule
from the cache as a **full clone without alternates** (D2), resets `origin` to the real URL,
applies V1's three-path branch resolution (K2), registers aliases, and **auto-commits** the
structural change (D13′).

`open` (re-materialization) clones each submodule from the bare cache by temporarily pointing
`submodule.<name>.url` at the cache, then restores the real URL and `origin` (verified offline,
no alternates). A freshly updated submodule is in detached HEAD (F9), so `open` attaches
branches by mode (D39):

| Mode | Checkout after `open` |
|---|---|
| `work`, `follow` | local branch named after `.gitmodules` `branch`, at the gitlink SHA; upstream `origin/<branch>` if it exists |
| `pin` | detached at the gitlink SHA |

`open` never resets a local branch that already exists.

### Commit and sync (D13′, C1–C4)

- **`commit [-m]`** stages root files and current gitlinks and commits on `task/<id>`. It
  never commits inside submodules and warns about dirty ones.
- **`sync`** (M5), in order:
  1. Pull tasktree state: `main` (semantic merge) and `task/<id>` (fast-forward only; on
     divergence stop and hand over to `resolve`).
  2. Apply gitlinks pulled from the other machine to clean submodules on their branch.
  3. Update repos by mode (D14).
  4. Auto-commit gitlink moves caused by step 3.
  5. Push tasktree state.
  6. Report dirty, diverged, and unpushed (gitlinks not reachable from `origin`, D13 O1).
- **`sync --no-repos`** skips steps 2–4: just move your files and records.
- **`sync --all`** syncs `main` and every `task/*` ref between hub and remote; it never touches
  open clones' working trees and reports which are behind.
- **`sync` never pushes code to `origin`.** Only `tasktree push` does (C4, Q14).
- `sync` does not commit implicitly; it warns about an uncommitted root.

### Cross-machine guarantee (D13 O1)

Only pushed work reproduces. A gitlink unreachable from `origin` falls back to the
`.gitmodules` branch tip on the other machine and is reported. Dirty state never crosses
machines.

### Lifecycle and close (D18)

`mark done|dropped` on a checked-out task:

1. **Gate:** refuse if any submodule is dirty or has commits unreachable from `origin` (they
   would be destroyed with the clone git dir). `--force` lists the loss and proceeds.
2. Auto-commit the root (`close: done`).
3. Sync `task/<id>` and `main` (`--no-repos`; hub-only until a remote exists).
4. Delete the working dir, the clone git dir, and the `local.yml` entry.
5. Keep `task/<id>` forever. Reopen + `open` re-materializes.

`--keep` changes status without teardown. Other machines' checkouts of closed tasks are
reported by `sync --all` and removed only on request.

---

## Subtasks

### Shared vs isolated (D29′, D30)

A subtask is a full task with `parent: <id>`. Like every task it has 0..1 tasktrees.

- **Shared (default):** `new --parent P "title"` creates only a record; work happens in the
  parent's tasktree.
- **Isolated:** `new --parent P --isolated` (or later `open <subtask>`, which promotes a shared
  subtask) creates its own tasktree.

```bash
cd ~/Developer/tasks/DSA-8946
tasktree new --parent DSA-8946 "Add retry to client"   # handle: add-retry
tasktree focus add-retry
tasktree note "retry budget = 3"     # → "note added to add-retry"
tasktree mark done
tasktree focus --clear
```

**Target resolution inside a tasktree:** `-t <handle>` → `TASKTREE_TASK` env → focused
subtask (per checkout, `local.yml`) → the tasktree's own task. Every write prints its target.
This applies to record commands; structural commands (`add`, `remove`, `commit`, `mode`,
`push`, `bootstrap`) always act on the tasktree root's own task (D45).
Parallel agents in one folder set `TASKTREE_TASK` instead of sharing focus.

### Isolated subtask tasktrees (D31, D32)

- **Contents:** the parent's `work` repos by default; `--repos a,b,c` selects explicitly,
  including `follow`/`pin` repos. Modes are copied. Root files are not copied; a fresh starter
  `AGENTS.md` points to the parent (`tasktree info -t <parent>`).
- **Branch name:** `<parent-branch>-<sub-handle>` (e.g. `feature/x-add-retry`); `--branch`
  overrides. `feature/x/add-retry` is impossible while `feature/x` exists (ref file/directory
  conflict, F5).
- **Fork point:** the parent's live local HEAD. The CLI adds a per-machine remote `parent`
  pointing at the parent's submodule git dir (`clones/<parent-id>.git/modules/<name>`) and
  fetches locally — no push needed, unpushed parent work included (F6). If the parent repo is
  dirty, fork from its last commit and warn.
- Inside the subtask's submodule, `git fetch parent` + merge pulls newer parent work with
  plain git.

### Integration (D33, D33a)

Isolated subtask work returns via `origin` only: `tasktree push` in the subtask, then a PR with
base `<parent-branch>` (manual `gh pr create`; no PR automation in V2.0).

`push` checks that `origin/<parent-branch>` contains the subtask's fork point. If not, it warns
("push DSA-8946's api first, or the PR will include 4 parent commits") and prints the matching
`gh pr create --base … --head …` line.

### Status rules (D34)

- **ST1:** closing a parent with open subtasks is refused (lists them). `--cascade` closes them
  too — shared with the same status, isolated each through the D18 gate; any refusal aborts
  the whole cascade before changes.
- **ST2:** no automatic rollup; `info`/`list` show progress (`3/5 done`).
- **ST3:** unlimited depth; rules apply recursively (`feature/x-add-retry-tests`).
- **ST4:** reparenting (`info set parent <h>` / `--clear`) changes only the record; `doctor`
  reports branch names no longer matching the parent.

---

## CLI (D27, D35)

Targeting: inside a tasktree, commands act on its task (see target resolution); anywhere,
`-t/--task <handle|id>`. Global `-C` and `-v` retained (K12). No command is named `status`.

| Code | Command | Purpose |
|---|---|---|
| X1 | `setup`, `config get\|set` | First run (remote, tasksRoot, author, machine) and machine config (D19) |
| X2 | `new "title" [--jira K] [--handle h] [--template t k=v] [--open]` | Create a task record, optionally with a tasktree |
| X3 | `list [--status] [--label] [--ref] [--all]` | Open tasks by default; marks tasks checked out here |
| X4 | `info [--json]` | Overview: record summary + repo table (mode, branch, HEAD, dirty, ahead/behind, unpushed) + root dirty state |
| X5 | `info edit`, `note "…" [--kind]`, `ref add\|rm`, `label add\|rm`, `history` | Record writes and git-derived changelog |
| X6 | `mark <status> [--keep] [--force] [--cascade]` | Set status; closed status runs the close flow (D18, ST1) |
| X7 | `open <handle> [--path]`, `root` | Materialize here or print path (`cd "$(tasktree open x)"`) |
| X8 | `add`, `remove`, `mode <repo> <mode>`, `bootstrap` | Tasktree structure; `remove` uses the D18 gate |
| X9 | `commit [-m]`, `sync [--no-repos] [--all]`, `push [repo…]` | Record and move state; `push` refuses protected branches (`main`, `master`, `release/*`) |
| X10 | `resolve [--take local\|remote <path>]` | Root divergence across machines (replaces git in the root, P1) |
| X11 | `alias add\|rm\|list`, `template list\|show\|validate` | Aliases and templates |
| X12 | `import`, `move <path>`, `doctor [--fix]` | Migration, relocation (R1), health checks (R1, R2, R4, handle collisions) |
| X13 | `new --parent <h> [--isolated] [--repos …] [--branch …]` | Create a subtask |
| X14 | `focus <h>`, `focus --clear` | Focus a subtask in this checkout |
| X15 | `open <subtask>` | Promote a shared subtask to isolated |
| X16 | `list` tree, `info` subtasks | Hierarchy and progress |
| X17 | `mark … --cascade` | Close a parent with its subtasks |
| X18 | `info set parent <h>` / `--clear` | Reparent |

### Agent integration (D26)

- **AG1:** every read command supports `--json` with a documented, stable shape; distinct
  exit codes for not-in-tasktree, conflict, gate-refused, not-found.
- **AG2:** author = `--author` → `TASKTREE_AUTHOR` → `config.yml` `author`; machine from config.
- **AG3:** the tool never edits user-owned files. The default template writes a starter root
  `AGENTS.md` once ("use `tasktree info`/`note`/`ref add`; never git in the root"). The pi
  `tasktree` skill is rewritten for V2.
- **AG4:** `.tasktree/task.md` is regenerated on `open`, `sync`, and every record write in
  that tasktree.

---

## M5 — Remote sync spec (D51–D54)

| Code | Behavior |
|---|---|
| S1 | `tt2 config set remote <url>` adds `origin` to the hub and pushes `main` + all `task/*`; `config set remote ""` removes it. `tt2 setup --remote <url>` on a machine without a hub clones it (D51). |
| S2 | `sync` inside a tasktree: pull `main` (semantic merge) → pull `task/<id>` (fast-forward only) → apply pulled gitlinks to clean submodules → update repos by mode (D14) → auto-commit tool-caused gitlink moves → push → report. `--no-repos` skips only the D14 upstream updates (origin fetch + fast-forward of follow/work branches) and their auto-commit; applying a pulled definition (S6) always runs, because skipping it would let a later `tt2 commit` silently revert the other machine's gitlinks. |
| S3 | `sync --all` (and `sync` outside a tasktree): sync `main` + every `task/*` ref; never touches checkouts; reports checkouts that are behind and checkouts of closed tasks. |
| S4 | Semantic merge of `main` per `tasks/<id>.md`: log = union by entry id (timestamp order); labels/refs 3-way set merge; scalars + description 3-way, conflicts per D52; other files on `main` use git's 3-way merge (conflict stops the sync); delete vs modify keeps the modified version. Merge commit `sync: merge <machine>`. |
| S5 | Fetch outside the hub lock; merge and update refs inside it; push with `--force-with-lease` against the fetched ref; a rejected push retries the sync up to 3 times. |
| S6 | Gitlink apply: a submodule whose gitlink moved, clean and on its `.gitmodules` branch → fetch from `origin` and fast-forward to the gitlink; commit not on `origin` → skip and report. |
| S7 | Repo updates use bounded parallel fetch (`sync.jobs`, default 4); a failing repo is reported and the rest continue; exit 1 at the end if anything failed. |
| S8 | `task/<id>` divergence: stop, show both commit lists and differing paths; `tt2 resolve [--take local\|remote] [path…]` (all paths when none given; gitlinks are paths) → merge commit + push. Repo branches are never reset: a clean repo whose branch is ahead of / diverged from the chosen gitlink keeps its tip and the mismatch is reported (the root then shows the gitlink as modified until reconciled). |
| S9 | `--allow-unrelated` required for the first merge of `main` histories with no common commit. |
| S10 | `doctor`: `remote-unreachable`, `main-behind`/`main-ahead`, `conflict-flag` (warnings). |

## V1 carry-over (D22)

| Code | V1 feature | V2 |
|---|---|---|
| K1 | Repo aliases + auto-registration | Keep (`~/.tasktree/aliases.yml`) |
| K2 | `--branch`/`--from` three-path resolution | Keep; also derives the default mode |
| K3 | Bare cache manager | Keep |
| K4 | Bootstrap steps | Keep, in `.tasktree/config.yml`; run on `open` and `bootstrap` |
| K5 | Templates + variable engine | Keep, retargeted: produce a record + optional tasktree (repos with modes, files, symlinks) |
| K6 | Snapshot/restore | Drop from V2.0 |
| K7 | tasktree-engine design | Defer; input becomes a task branch ref |
| K8 | Annotations, labels | Replaced by record frontmatter |
| K9 | Registry, `list`, `prune` | Replaced by `main` + `local.yml`; `prune` → `doctor` orphan cleanup |
| K10 | `migrate` (`.tasktree.toml`) | Drop; superseded by `import` |
| K11 | http/archive/static/local materializers | Drop (D11) |
| K12 | `-C/--chdir`, `-v/--verbose` | Keep |
| K13 | `Tasktree.yml` JSON schema | Replace with schemas for record frontmatter and `.tasktree/config.yml` |

## Implementation strategy (D23)

- Same repo, Go, module `github.com/tinmancoding/tasktree/v2` on `main`; V1 frozen on a `v1`
  branch for fixes. Released as `v2.0.0` via release-please (`feat!:`); same binary name.
- **During development the V2 binary is `tt2`** (`cmd/tt2`), installed alongside V1 `tasktree`
  and its `tt` alias, so existing V1 workspaces keep working until `import` (M3). It takes the
  `tasktree` name at `v2.0.0` (D37, D42).
- Development happens on branch `v2`; V1 is frozen on branch `v1`; `v2` merges to `main` at
  `v2.0.0`.
- Rewrite `domain`, `app`, `cli` (~79% of V1 non-test code).
- Port leaf packages: `gitx` (extended for `GIT_DIR`/submodules), `output`, `bootstrap`,
  `variable`, `testutil`, `materialize/git.go` (K2), `repoalias`, `fsx`, `cache` (~1.8k LOC).
- V2 includes a read-only V1 `Tasktree.yml` reader for `import`.

## Migration from V1 (D24)

`tasktree import <v1-path> | --all [--move] [--dry-run]` adopts V1 tasktrees in place:

1. Create a record: handle = folder name, status `active`, V1 annotations/template info →
   `refs` and log.
2. Create orphan `task/<id>`.
3. Convert each existing clone to a submodule by moving its `.git` into
   `clones/<id>.git/modules/<name>` (atomic rename on the same APFS volume). No recloning;
   branches, unpushed commits, dirty state, and stashes are preserved. Branch and mode come from
   the checkout's **current** state, not the V1 spec (D43): on the default branch → `follow`,
   on another branch → `work`, detached → `pin`.
4. Commit root artifacts; convert `local` sources to symlinks; remove `Tasktree.yml`.
5. Verify `git status --porcelain` and `HEAD` per repo before/after; any difference rolls back.

The folder stays in place (path in `local.yml`) unless `--move` relocates it to
`<tasksRoot>/<handle>`.

## Milestones (D28, D35)

| Milestone | Scope |
|---|---|
| **S — Walking skeleton** (D41) | Hub, `setup`, `new` (incl. `--parent` for shared subtasks), `list`, `info`, `note`, `mark` (incl. close flow), `open`, `add` (aliases, modes, K2), `remove`, `commit`, `root`, `-t` targeting, `--json` for `list`/`info`. Record syntax (D38), `open` branch rules (D39), git env/path rules (D40). Binary `tt2`. Backup of `~/.tasktree` in place (R4). |
| **M1 — Records** | `info edit`, `info set` (incl. reparent), `ref`, `label`, `history`, `list` tree, `mark --cascade`, starter `AGENTS.md`. |
| **M2 — Tasktrees + isolated subtasks** | `mode`, `bootstrap`, `focus` (D36), full target resolution (`TASKTREE_TASK`, focus), `.tasktree/task.md`, pi skill rewrite, `--isolated`, promotion, `parent` remote, `push` + fork-point check. |
| **M3 — Import** | `import` (D24); stop using V1 day to day. |
| **M4 — Polish** | Retargeted templates, `doctor` (incl. orphan cleanup, moved from M2), `move`. |
| **M5 — Remote** | `sync`, `sync --all`, `resolve`, semantic merge. |

## Deferred

- Jira (or other tracker) import as an add-on (`tasktree jira pull`, D25 O2).
- Local merge path for isolated subtasks (`tasktree merge`, D33 I1).
- Snapshot/restore and tasktree-engine (K6, K7).
- Automatic/background sync.
- Tasks remote hosting choice (D20: local-only for now).

## Risks

| Code | Risk | Mitigation |
|---|---|---|
| R1 | Moving a task folder breaks submodule `.git` files and `core.worktree` | Absolute paths; `move` and `doctor --fix` rewrite them |
| R2 | Deleting a folder by hand leaves an orphan clone git dir | `close` cleans both; `doctor` finds orphans |
| R3 | Tools that honour `.gitignore` only inside a repo do not treat the root as a repo | Accepted; consistent with P1 |
| R4 | No remote: `~/.tasktree` is the only copy of all records and artifacts | Backup from M1; `doctor` warns when no remote is configured |

## Verified git facts

Checked against git 2.53.0:

- **F4:** a branch can be checked out in only one worktree; submodule git dirs inside worktrees
  are not shared; `submodule add --reference` creates alternates to the cache; a submodule
  commit shows the superproject as modified.
- **Hidden root:** with `--separate-git-dir` and the root `.git` removed, `git status` in the
  root fails with "not a git repository", git inside a submodule works, and root operations
  work via `GIT_DIR`/`GIT_WORK_TREE`.
- **F5:** `refs/heads/feature/x` and `refs/heads/feature/x/sub` cannot coexist.
- **F6:** unpushed parent commits exist only in the parent clone's submodule git dir.
- **End-to-end M2 path:** clone from the bare cache → `submodule add` of the existing clone →
  `submodule absorbgitdirs` into the hidden clone git dir (no alternates) → commit → push to hub
  → upstream taken offline → fresh hidden clone of `task/<id>` → `submodule update` from the
  cache → real URL restored. Root clean; plain `git status` in the root fails as intended.
- **F7:** exported `GIT_DIR`/`GIT_WORK_TREE` leak into child git processes (`git clone` fails).
- **F8:** git writes relative paths for submodule `gitdir` and `core.worktree`.
- **F9:** a submodule populated by `submodule update` is in detached HEAD.

---

## Decision log

| Code | Decision |
|---|---|
| D1 | Single-person system; multi-machine use later; optimistic concurrency across machines |
| D2 | No `git worktree`; submodules are full clones from the bare cache, no alternates |
| D3 | Local hub + one full clone per open task; only the hub talks to the remote |
| D4 | Task is the primary entity on `main`; 0..1 tasktree per task; `parent` field for subtasks |
| D5 | Immutable random ID + mutable handle; branch `task/<id>`, directory `<handle>` |
| D6 | One record file per task: `tasks/<id>.md` on `main` |
| D7 | `main` is the single source of truth for records; CLI (`info …`) is the interface; `.tasktree/task.md` optional |
| D8 | Record = typed frontmatter + mutable Description + append-only Log; changelog from git |
| D9 | Hub is non-bare at `~/.tasktree/git`; local writes via `flock` + porcelain; optimistic only at `sync` |
| D10 | Task branches are orphans |
| D11 | Tasktree branch = submodules + committed files + symlinks; http/archive/static removed |
| D12 | Mandatory committed `.tasktree/config.yml` (root marker, `taskId`, optional `bootstrap`) |
| D13′ | `commit` records root state; `sync` moves committed state; structural changes auto-commit |
| D14 | Per-repo modes `follow`/`work`/`pin`; the tool commits gitlink moves it caused |
| D15 | Hidden root git dir at `~/.tasktree/clones/<id>.git`; no `.git` in the root |
| D16′ | Open: `incoming`, `next`, `active`, `blocked`; closed: `done`, `dropped` |
| D17 | Checkout state is per machine in `~/.tasktree/local.yml`, never on `main` |
| D18 | Close flow: gate, auto-commit root, sync, teardown, keep branch; `--keep`, `--force` |
| D19 | Remote is configuration (`setup`, `config set remote`); none is valid |
| D20 | Laci's instance starts local-only |
| D21 | Layout: `~/.tasktree` (L3, `TASKTREE_HOME`), cache in OS cache dir (L2), flat task dirs (L1) |
| D22 | V1 carry-over table K1–K13 |
| D23 | Rewrite in the same repo as Go module `/v2`; port leaf packages |
| D24 | In-place `import` with verify-or-rollback |
| D25 | External trackers via typed refs only |
| D26 | Agent integration AG1–AG4 |
| D27 | CLI surface X1–X12; `info` = overview, `mark` = status; no `status` command; code reaches `origin` only via `push` |
| D28 | Milestones M1 → M5; sync last |
| D29′ | Subtasks share the parent's tasktree by default; `--isolated`/`open` for their own |
| D30 | Target resolution: `-t` → `TASKTREE_TASK` → focus → tasktree's task; writes print target |
| D31 | Isolated subtask copies parent's `work` repos by default (`--repos` to choose) |
| D32 | Isolated branch `<parent-branch>-<sub-handle>`, forked from parent's live HEAD via local `parent` remote |
| D33 | Isolated subtask work integrates via `origin` PRs only; `push` checks the fork point (D33a) |
| D34 | Status rules ST1–ST4 (cascade, no rollup, unlimited depth, record-only reparent) |
| D35 | Subtask milestones: shared in S/M1, isolated + `push` in M2 (revised by D41) |
| D36 | `focus` moves to M2 (it is per checkout); before that, record commands take the task as an argument or `-t` |
| D37 | Development binary is `tt` alongside V1 `tasktree`; renamed to `tasktree` at `v2.0.0` |
| D38 | Record syntax: reserved `## Description`/`## Log`, Log last, entry header `### <ts> <kind> <author>@<machine> {#e-<id>}`, lossless parser |
| D39 | `open` attaches `work`/`follow` repos to a local branch at the gitlink (tracking `origin/<branch>` if present), `pin` detached; never resets existing branches |
| D40 | Never export `GIT_*`; strip inherited repository-context `GIT_*` (`git rev-parse --local-env-vars`) for child processes, keep auth/transport vars; rewrite submodule paths to absolute |
| D41 | Walking-skeleton milestone S precedes M1; M1/M2 reduced accordingly |
| D42 | Development binary is `tt2` (revises D37); `tt` stays the V1 alias |
| D43 | `import` derives branch and mode from each checkout's current state, not the V1 spec; orphan cleanup moves from M2 to M4 (`doctor`); M3 rollout: dry-run → backup → one small workspace → `--all` |
| D44 | `push` without arguments pushes only `work` repos and checks protected branches for all selected repos before pushing any; `follow`/`pin` repos are pushed only when named |
| D45 | Structural commands (`add`, `remove`, `commit`, `mode`, `push`, `bootstrap`) always act on the tasktree root's own task; `focus`/`TASKTREE_TASK` retarget only record commands (refines D30) |
| D46 | The loss gate (close, `remove`, import report) counts commits reachable from HEAD or local branches but not from `origin`, plus stashes; tags are excluded (tags fetched from `origin` are indistinguishable from local ones and caused false refusals) |
| D47 | Three concepts: **task** (record on `main`), **tasktree** (definition: the `task/<id>` branch with sources, modes, files; may exist without a checkout), **checkout** (materialization on one machine). Tasks never hold sources. Definition edits (`add/remove/mode -t`, templates) work without a checkout by writing the hub branch directly (hub lock, temporary index, `commit-tree`, compare-and-swap `update-ref`); base commits resolve from the bare cache (fetch, no clone). With a local checkout, the checkout path is used |
| D48 | `tt2 new` creates only a record (plus optional `--open`); repos come from `add -t` or a template |
| D49 | Template = `task:` (record fields) + optional `tasktree:` (`repos`, `symlinks`, `files`, `bootstrap`) → record + tasktree definition; V1 parameters and `{{var}}` syntax kept |
| D50 | Templates TP1–TP6: only `~/.tasktree/templates/` + builtins (`bugfix`, `feature`, `investigation`; user overrides builtin); `aibm-general` converted by hand; `template list/show/validate`; V1 variable precedence, provenance in the first log entry; `--template` with `--parent` only for record-only templates |
| D51 | A second machine can always join: `setup --remote` clones the hub when none exists; the first merge of unrelated `main` histories requires `sync --allow-unrelated`, then the semantic merge applies (random ids → no file collisions; duplicate handles reported by `doctor`) |
| D52 | Description and scalar-field conflicts keep the local value, log the remote value as a `comment` entry, and set frontmatter `conflict: <field>` (shown by list/info/doctor; cleared by `info edit`/`info set`); labels and refs merge as 3-way sets |
| D53 | Only `open` (fetch `task/<id>` + `main` when missing/behind) and closing (push `task/<id>` + `main`) touch the remote implicitly, best-effort; all other writes wait for `sync`; `sync` outside a tasktree = `sync --all` |
| D54 | M5 spec S1–S10 (remote config/join, sync steps, `--all`, semantic merge, lease + retry, gitlink apply, parallel repo updates, `resolve`, `--allow-unrelated`, doctor checks) as recorded in the M5 design section |
| D55 | Sync conflict comments identify the remote side by the last remote commit touching the file (`commit <sha> by <author> at <date>`); no machine metadata is persisted; `sync: merge <machine>` names the local machine. `--no-repos` and `resolve` semantics as refined in S2/S8 |
| D56 | `resolve` is 3-way against the merge base: one-sided changes merge automatically, `--take` decides only paths changed on both sides. Pulled `pin` gitlinks apply as a detached checkout; previously skipped gitlinks are re-applied by the next `sync`. `commit` and close refuse to move a gitlink backwards (`--allow-rewind` / `mark --force` override). Network calls are bounded by timeouts and never prompt |
| D57 | V2 documentation replaces the V1 docs on `main`; V1 docs stay on the `v1` branch and the README links to them. V1 design docs move to `docs/design/archive/` |
| D58 | Audit decisions DN1–DN10: build `alias add/rm/list` and `list --ref`; `--json` for `template list/show` and documented JSON shapes; `bootstrap add/rm/list` (also via `-t`); `open` never falls back to a branch tip for an unavailable gitlink (repo left uninitialized + reported, retried by `sync`/`open`); `open` on an existing checkout only prints its path; `--template <file>` allowed explicitly; `setup --remote` on an existing hub pushes like `config set remote`; at `v2.0.0` the binary is `tasktree` with `tt` and `tt2` installed as symlinks, new text says `tasktree`; `status` removed from `info set` (`mark` is the only status writer) and `info ack <field>` clears conflict flags |
| D59 | Network policy (fixes M19): explicit transfers (`add`, `open` materialization, `push`, `sync`, `resolve`, `setup`, `config set remote`) have no wall-clock limit and use stall detection (SSH `ConnectTimeout=15`, `ServerAliveInterval=15`, `ServerAliveCountMax=4`; HTTP `lowSpeedLimit=1000`, `lowSpeedTime=60`); implicit best-effort calls (open pre-fetch, close push, doctor) keep a short wall clock (10s); prompts always disabled |
