# Overview

## What is tasktree?

Tasktree is a CLI tool for managing task-focused multi-repository workspaces. It solves a specific problem: when a feature, bugfix, or investigation spans multiple repositories, you need a way to collect them into one workspace, checkout the right branches, and hand that setup off to teammates or CI.

A **tasktree** is a directory on disk that contains:

- A `Tasktree.yml` file — the declarative spec describing which repositories (and other sources) belong to this workspace and at what ref
- One or more checkout directories — the actual clones materialized from the spec

```
feature-payments/
├── Tasktree.yml          ← the spec (commit this)
├── api/                  ← cloned from github.com/myorg/api
├── web/                  ← cloned from github.com/myorg/web
└── payments-service/     ← cloned from github.com/myorg/payments-service
```

## The core workflow

```
tasktree init             # create Tasktree.yml
tasktree add <repo-url>   # clone + add to spec
tasktree add <repo-url>   # repeat for each repo
git add Tasktree.yml && git commit

# teammate or CI:
tasktree apply            # materialize everything declared in the spec
```

## Key design decisions

**`Tasktree.yml` is pure desired state.** It stores intent — which URL, which branch, which ref — not resolved state like commit SHAs. Live information (current HEAD, dirty status) is always read directly from the Git working copies by `tasktree status`.

**Cloning is cached.** Each unique repository URL is cloned once into a bare-repo cache at `~/.cache/tasktree/repos/<sha256-of-url>`. Subsequent clones of the same URL (in any workspace) use the cache, making `add` and `apply` fast.

**The spec is versionable.** `Tasktree.yml` is designed to be committed to a git repo. This makes workspaces reproducible and shareable — a teammate runs `tasktree apply` and gets the same setup.

**Context resolution.** Most commands (`add`, `remove`, `status`, `repos`) resolve the tasktree root by walking up from the current directory until a `Tasktree.yml` is found. You don't need to `cd` to the root.

## Global registry

Tasktree maintains a global registry at `~/.local/state/tasktree/registry.toml` that tracks all known tasktrees on the machine. This powers `tasktree list` and can be cleaned up with `tasktree prune`.
