# Hotfix Workflow

When you need to investigate or patch a specific release, use `--from` with a tag or commit SHA to check out a pinned, read-only state.

## Scenario

Production is running `api` at `v2.3.1`. You need to investigate a bug and potentially cut a patch release.

## Setup

```bash
mkdir ~/work/hotfix-login && cd ~/work/hotfix-login
tasktree init
```

### Option A: Pinned tag, no branch (investigation only)

Check out the exact release tag without creating a branch:

```bash
tasktree add git@github.com:myorg/api.git --from v2.3.1
```

The repo is in a detached HEAD state at `v2.3.1`. Good for read-only investigation.

### Option B: New hotfix branch from a tag

Create a `hotfix/login-bug` branch starting from `v2.3.1`:

```bash
tasktree add git@github.com:myorg/api.git --branch hotfix/login-bug --from v2.3.1
```

This creates `hotfix/login-bug` branching off the `v2.3.1` tag. You can commit, push, and open a PR.

## Verify the starting point

```bash
tasktree status
```

```
Tasktree: hotfix-login  /home/user/work/hotfix-login

NAME    PATH    HEAD              STATE
api     api     hotfix/login-bug  clean
```

## Tip: multiple services at a pinned state

If the bug spans multiple services, pin each to its corresponding release tag:

```bash
tasktree add git@github.com:myorg/api.git --branch hotfix/login-bug --from v2.3.1
tasktree add git@github.com:myorg/auth.git --branch hotfix/login-bug --from v1.8.0
```
