# tasktree add

Add a repository to the current tasktree.

## Synopsis

```
tasktree add <repo-url> [flags]
```

## Description

Clones the repository into a subdirectory of the tasktree root, appends a source entry to `Tasktree.yml`, and registers derived aliases for the repo URL.

Cloning uses the bare-repo cache at `~/.cache/tasktree/repos/<sha256-of-url>`. The first `add` of a URL fetches from the network; subsequent adds of the same URL (in any workspace) use the cache.

`<repo-url>` may be a full clone URL or a [repository alias](../concepts/repo-aliases.md).

## Arguments

| Argument | Description |
|---|---|
| `repo-url` | Clone URL or alias. Supports SSH (`git@host:org/repo.git`) and HTTPS. |

## Flags

| Flag | Description |
|---|---|
| `--branch <name>` | Branch to use (see branch resolution below). |
| `--from <ref>` | Base ref for branch creation, or direct checkout when `--branch` is omitted. |
| `--name <name>` | Override the checkout directory name. Defaults to the repo basename. |

## Branch resolution

The combination of `--branch` and `--from` covers several workflows:

### Default branch (no flags)

```bash
tasktree add git@github.com:myorg/api.git
```

Checks out the remote default branch (e.g., `main`).

### Checkout a specific branch

```bash
tasktree add git@github.com:myorg/api.git --branch feature/payments
```

Resolution order:

1. If `feature/payments` exists **locally** → checks it out, `--from` is ignored if provided
2. If `feature/payments` exists **on origin** → creates a local tracking branch
3. If it exists **nowhere** → creates a new branch from `--from` (or default branch if omitted)

### Create a new branch from a base ref

```bash
tasktree add git@github.com:myorg/api.git --branch feature/payments --from main
```

Creates `feature/payments` branching off `main`. If `feature/payments` already exists locally or on origin, `--from` is ignored.

### Check out a specific ref without creating a branch

```bash
tasktree add git@github.com:myorg/api.git --from v1.4.0
```

Checks out the tag or SHA directly (headless). Use this for pinned, read-only checkouts.

### Override the checkout directory name

```bash
tasktree add git@github.com:myorg/payments-service.git --name payments
```

Clones into `payments/` instead of `payments-service/`.

## Output

```
Using existing remote branch "feature/payments" from origin.
Added api at api
Registered alias api -> git@github.com:myorg/api.git
Registered alias myorg/api -> git@github.com:myorg/api.git
```

## Errors

| Error | Cause |
|---|---|
| `duplicate repo name: <name>` | A source with this name or path already exists in `Tasktree.yml`. |
| `destination already exists: <path>` | The checkout directory already exists on disk. |
| `invalid repo name: <name>` | The derived or provided name contains invalid characters. |
| `unresolved ref <ref> in <url>` | The `--from` ref cannot be resolved in the repository. |
| `invalid branch name: <name>` | The branch name fails `git check-ref-format`. |
