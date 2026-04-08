# Multi-Service Feature Branch

The most common tasktree workflow: a feature that touches multiple services simultaneously, each needing the same branch checked out.

## Scenario

You're building a payments flow that requires changes to `api`, `web`, and `payments-service`. All three need to be on `feature/payments`.

## Setup

```bash
mkdir ~/work/feature-payments && cd ~/work/feature-payments
tasktree init

tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/web.git --branch feature/payments
tasktree add git@github.com:myorg/payments-service.git --branch feature/payments
```

On the first `add`, if `feature/payments` doesn't exist yet, tasktree creates it from the default branch. On subsequent adds to other repos, the same happens — each gets a fresh `feature/payments` branch.

If the branch already exists on origin (e.g., a teammate pushed it), tasktree creates a local tracking branch automatically.

## Share the spec

The workspace directory already contains cloned git repositories as subdirectories — you cannot `git init` in that same directory and version-control `Tasktree.yml` there, because git won't track the contents of the nested repo subdirectories.

Instead, keep `Tasktree.yml` separate from the clones:

**Option A: dedicated spec repo**

Keep a separate repository that contains only `Tasktree.yml` files for your workspaces:

```bash
# in a separate directory, not inside ~/work/feature-payments
cd ~/repos/workspaces
cp ~/work/feature-payments/Tasktree.yml feature-payments.yml
git add feature-payments.yml
git commit -m "Add payments workspace spec"
git push
```

**Option B: share as a file**

Paste `Tasktree.yml` into a GitHub Gist, PR description, Confluence page, or send it directly to teammates.

## Teammate reproduces the workspace

```bash
mkdir ~/work/feature-payments && cd ~/work/feature-payments
# copy or download Tasktree.yml into this directory
tasktree apply
```

## Check everything at a glance

```bash
tasktree status
```

```
Tasktree: feature-payments  /home/user/work/feature-payments

NAME              PATH              HEAD              STATE
api               api               feature/payments  dirty
web               web               feature/payments  clean
payments-service  payments-service  feature/payments  clean
```

`api` has uncommitted changes — time to commit before the standup.
