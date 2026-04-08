# Workspace Spec (Tasktree.yml)

`Tasktree.yml` is the declarative specification of a tasktree workspace. It is written by the CLI tools (`init`, `add`, `remove`) and read by `apply`, `repos`, and `status`. You should commit it to version control.

## Structure

```yaml
apiVersion: tasktree.dev/v1
kind: Tasktree
metadata:
  name: feature-payments
  description: "Payment flow across api, web, and payments-service"
  createdAt: "2026-03-25T12:00:00Z"
spec:
  sources:
    - name: api
      type: git
      path: api
      git:
        url: git@github.com:myorg/api.git
        ref: feature/payments
        branch: feature/payments

    - name: web
      type: git
      path: web
      git:
        url: git@github.com:myorg/web.git
        branch: feature/payments
```

## Top-level fields

| Field | Required | Description |
|---|---|---|
| `apiVersion` | yes | Must be `tasktree.dev/v1` |
| `kind` | yes | Must be `Tasktree` |
| `metadata` | yes | Name, description, timestamps, labels |
| `spec` | yes | The desired workspace state |

## metadata

| Field | Required | Description |
|---|---|---|
| `name` | yes | Human-readable workspace name. Alphanumeric, hyphens, underscores. Set by `init` from the directory name. |
| `description` | no | Free-text description of the task or purpose |
| `createdAt` | no | RFC3339 timestamp. Set automatically by `init`, do not edit. |
| `labels` | no | Arbitrary key/value string pairs for tooling or filtering |

## spec.sources

An ordered list of sources to materialize. Each entry is a `SourceSpec`:

| Field | Required | Description |
|---|---|---|
| `name` | yes | Logical name for this source within the workspace. Used by `remove`. Must be unique. |
| `type` | yes | Source kind: `git`, `http`, `archive`, `static`, or `local`. Only `git` is currently implemented. |
| `path` | no | Relative path inside the workspace directory. Defaults to `name`. |
| `git` | conditional | Required when `type: git`. See below. |

## git source

```yaml
git:
  url: git@github.com:myorg/api.git   # required
  ref: feature/payments               # optional
  branch: feature/payments            # optional
```

| Field | Description |
|---|---|
| `url` | Git clone URL. SSH (`git@host:org/repo.git`) and HTTPS (`https://host/org/repo.git`) are both supported. |
| `ref` | The base ref or explicit checkout target. Used by `apply` as the base when creating branches. |
| `branch` | The branch to check out or create. If it exists locally, it is reused. If it exists only on origin, it is tracked. If it doesn't exist, it is created from `ref`. |

### Branch vs ref

The fields are written by `tasktree add` based on what you passed:

- `--branch feature/x` sets `branch: feature/x`
- `--from main` sets `ref: main`
- Both together: `ref: main`, `branch: feature/x`
- Neither (default branch): both fields omitted, the default branch is resolved at apply time

## JSON Schema

A JSON Schema for editor validation and linting is at `schema/tasktree.schema.json`. Configure your editor to use it for `Tasktree.yml` files.

### VS Code

Add to `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "./schema/tasktree.schema.json": "Tasktree.yml"
  }
}
```
