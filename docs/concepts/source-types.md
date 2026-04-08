# Source Types

The `type` field in a `SourceSpec` determines what gets materialized and how. The schema defines five types; only `git` is currently implemented.

## git (implemented)

Clones a Git repository into the workspace directory.

```yaml
- name: api
  type: git
  path: api
  git:
    url: git@github.com:myorg/api.git
    ref: main
    branch: feature/payments
```

Cloning is accelerated by a per-URL bare-repo cache at `~/.cache/tasktree/repos/<sha256-of-url>`. The first clone of a URL populates the cache; subsequent clones (in any workspace) use the cache and only fetch new objects.

See [Workspace Spec](./workspace-spec.md) for full `git` field documentation.

## http (not yet implemented)

Downloads a single file from an HTTPS URL and places it at `path`.

```yaml
- name: base-config
  type: http
  path: config/base.json
  http:
    url: https://example.com/config/base.json
    sha256: e3b0c44298fc1c149afbf4c8996fb924...
```

| Field | Description |
|---|---|
| `url` | HTTPS URL to download. HTTP is not permitted. |
| `sha256` | Expected SHA-256 hex digest. If provided, the download is verified. Strongly recommended. |
| `headers` | Optional HTTP request headers (e.g., `Authorization`). |

## archive (not yet implemented)

Downloads a remote archive (tarball or zip) and extracts it into `path`.

```yaml
- name: contracts
  type: archive
  path: contracts
  archive:
    url: https://github.com/myorg/contracts/archive/refs/tags/v1.4.0.tar.gz
    sha256: abc123...
    stripComponents: 1
```

| Field | Description |
|---|---|
| `url` | HTTPS URL of the archive. |
| `sha256` | Expected SHA-256 hex digest of the archive file. |
| `format` | `tar.gz`, `tar.bz2`, `tar.xz`, or `zip`. Inferred from the URL if omitted. |
| `stripComponents` | Number of leading path components to strip on extraction (like `tar --strip-components`). Default `0`. |

## static (not yet implemented)

Writes inline content from `Tasktree.yml` directly to a file at `path`.

```yaml
- name: docker-compose-override
  type: static
  path: docker-compose.override.yml
  static:
    content: |
      services:
        api:
          environment:
            DEBUG: "true"
    mode: "0644"
```

| Field | Description |
|---|---|
| `content` | Literal file content. Use YAML block scalars (`\|` for literal, `>` for folded). |
| `mode` | Unix file permission mode in octal string notation. Default `0644`. |

## local (not yet implemented)

Links or copies a local filesystem path into the workspace.

```yaml
- name: shared-scripts
  type: local
  path: scripts
  local:
    sourcePath: /home/user/shared-scripts
    copy: false
```

| Field | Description |
|---|---|
| `sourcePath` | Absolute or workspace-relative path to the local source. |
| `copy` | If `true`, copy instead of symlink. Default `false`. |
