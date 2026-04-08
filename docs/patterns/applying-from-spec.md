# Sharing a Workspace with apply

`Tasktree.yml` is designed to be shared and reproduced with `tasktree apply`. This pattern covers the full lifecycle.

## Important: don't mix the spec with the checkouts

The workspace directory contains cloned git repositories as subdirectories. You cannot `git init` in that same directory and track `Tasktree.yml` there — the nested repo subdirectories won't be tracked by the outer repo.

Keep `Tasktree.yml` separate from the workspace directory.

## Create the spec

```bash
mkdir ~/work/feature-auth && cd ~/work/feature-auth
tasktree init
tasktree add git@github.com:myorg/api.git --branch feature/auth
tasktree add git@github.com:myorg/web.git --branch feature/auth
```

The generated `Tasktree.yml`:

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree
metadata:
  name: feature-auth
  createdAt: "2026-04-08T09:00:00Z"
spec:
  sources:
    - name: api
      type: git
      path: api
      git:
        url: git@github.com:myorg/api.git
        ref: feature/auth
        branch: feature/auth
    - name: web
      type: git
      path: web
      git:
        url: git@github.com:myorg/web.git
        ref: feature/auth
        branch: feature/auth
```

## Share the spec

**Option A: dedicated spec repo**

Store `Tasktree.yml` in a separate repository that contains only workspace specs — not in the workspace directory itself:

```bash
cd ~/repos/workspaces
cp ~/work/feature-auth/Tasktree.yml feature-auth.yml
git add feature-auth.yml
git commit -m "Add feature-auth workspace spec"
git push
```

**Option B: share as a file**

Copy the contents of `Tasktree.yml` into a GitHub Gist, PR description, Confluence page, or send it directly.

## Teammate reproduces the workspace

```bash
mkdir ~/work/feature-auth && cd ~/work/feature-auth
# copy or download Tasktree.yml into this directory
tasktree apply
```

Output:

```
Using existing remote branch "feature/auth" from origin.
Cloned api at api
Using existing remote branch "feature/auth" from origin.
Cloned web at web
```

## Apply is idempotent

Running `apply` again on a fully materialized workspace is safe:

```bash
tasktree apply
# All sources are already present.
```

## Preview before applying

```bash
tasktree apply --dry-run
```

```
Would clone api at api (branch: feature/auth)
Would clone web at web (branch: feature/auth)
```

## CI usage

`apply` is well-suited to CI pipelines. Download `Tasktree.yml` from wherever you store it, then run `apply`:

```yaml
# Example GitHub Actions step
- name: Reproduce workspace
  run: |
    curl -L https://github.com/tinmancoding/tasktree/releases/latest/download/tasktree_Linux_amd64.tar.gz | tar xz
    sudo mv tasktree /usr/local/bin/
    mkdir workspace && cd workspace
    curl -o Tasktree.yml https://raw.githubusercontent.com/myorg/workspaces/main/feature-auth.yml
    tasktree apply
```

## Keeping the spec up to date

When you add a new repo to the workspace, copy the updated `Tasktree.yml` to wherever you're storing the spec and share it with the team. Teammates run `tasktree apply` to materialize the new source:

```bash
# you:
tasktree add git@github.com:myorg/notifications.git --branch feature/auth
cp ~/work/feature-auth/Tasktree.yml ~/repos/workspaces/feature-auth.yml
# commit/push or share the updated file

# teammate:
tasktree apply
# → Cloned notifications at notifications
```
