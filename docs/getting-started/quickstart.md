# Quickstart

This guide walks through creating a tasktree workspace and adding repositories to it in under five minutes.

## 1. Initialize a workspace

```bash
mkdir feature-payments && cd feature-payments
tasktree init
```

This creates `Tasktree.yml` in the current directory and registers the workspace in the global registry (`~/.local/state/tasktree/registry.toml`).

```
Initialized tasktree at /home/user/feature-payments
```

## 2. Add a repository

```bash
tasktree add git@github.com:myorg/api.git
```

Tasktree clones the repository into a `api/` subdirectory (derived from the URL) and updates `Tasktree.yml`:

```
Added api at api
Registered alias api -> git@github.com:myorg/api.git
```

To target a specific branch:

```bash
tasktree add git@github.com:myorg/api.git --branch feature/payments
```

If `feature/payments` already exists on origin, it is tracked. If it doesn't exist, it is created from the default branch. To base it on a different ref:

```bash
tasktree add git@github.com:myorg/api.git --branch feature/payments --from main
```

## 3. Check workspace status

```bash
tasktree status
```

Outputs a table showing each repository's current branch and working tree state:

```
Tasktree: feature-payments  /home/user/feature-payments

NAME    PATH    HEAD                STATE
api     api     feature/payments    clean
```

## 4. List repositories in the workspace

```bash
tasktree repos
```

Shows what is declared in `Tasktree.yml`:

```
NAME    PATH    REF                 BRANCH
api     api     feature/payments    feature/payments
```

## 5. Share the spec

`Tasktree.yml` is the shareable artifact. However, the workspace directory already contains cloned git repositories as subdirectories — you cannot `git init` in the same directory and track `Tasktree.yml` there, because the nested repos won't be tracked correctly by the outer repo.

The recommended ways to share a `Tasktree.yml`:

**Copy it into a dedicated repo** (separate from the workspace directory):

```bash
cp ~/work/feature-payments/Tasktree.yml ~/repos/workspaces/feature-payments.yml
# commit and push that repo separately
```

**Or share it as a file** — paste it into a GitHub Gist, a Confluence page, a PR description, Slack, etc.

A teammate can then reproduce the workspace by placing the file in an empty directory and running `apply`:

```bash
mkdir ~/work/feature-payments && cd ~/work/feature-payments
# copy or download Tasktree.yml here
tasktree apply
```

`apply` reads `Tasktree.yml` and materializes any sources not already present on disk.

## 6. Remove a repository

```bash
tasktree remove api
```

Deletes the `api/` checkout directory and removes the source entry from `Tasktree.yml`.

## Next steps

- [Basic Concepts](../concepts/overview.md) — understand what a tasktree is and how it works
- [CLI Reference](../cli/overview.md) — full flag and option documentation
- [Common Patterns](../patterns/multi-service-feature.md) — real-world workflows
