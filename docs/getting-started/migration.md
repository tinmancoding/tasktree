# Migrating from .tasktree.toml

Earlier versions of tasktree used a TOML-based metadata file called `.tasktree.toml`. The current version uses `Tasktree.yml`, a YAML-based declarative format.

If you have an existing workspace with `.tasktree.toml`, tasktree will detect it and prompt you to migrate:

```
Error: found legacy .tasktree.toml at /home/user/feature-payments — run 'tasktree migrate' to convert
```

## Running the migration

```bash
cd /path/to/workspace
tasktree migrate
```

Or pass the path explicitly:

```bash
tasktree migrate /path/to/workspace
```

The command reads `.tasktree.toml`, converts every repo entry to the new `SourceSpec` format, and writes `Tasktree.yml`. The old file is renamed to `.tasktree.toml.bak`.

Example output:

```
Found .tasktree.toml in /home/user/feature-payments.

Converting to Tasktree.yml...

  api                  git  git@github.com:myorg/api.git  (branch: feature/payments)
  web                  git  git@github.com:myorg/web.git

Note: resolved_ref and commit fields are not carried over.
      Live state is always queried from the Git checkouts directly.

Written: /home/user/feature-payments/Tasktree.yml
Renamed: .tasktree.toml → .tasktree.toml.bak

Migration complete. Review Tasktree.yml and commit it to version control.
```

## What changes

| Old field (`.tasktree.toml`) | New field (`Tasktree.yml`) |
|---|---|
| `[[repos]]` array | `spec.sources[]` array |
| `name` | `name` |
| `url` | `git.url` |
| `checkout` / `branch` | `git.ref` / `git.branch` |
| `resolved_ref`, `commit` | **Dropped** — live state is always read from the Git checkout |

The `resolved_ref` and `commit` fields are intentionally discarded. Tasktree no longer stores resolved state in the spec file; all live information is queried directly from the Git working copies.

## After migration

Review the generated `Tasktree.yml`, then commit it:

```bash
git add Tasktree.yml
git rm .tasktree.toml   # or keep the .bak for reference
git commit -m "Migrate to Tasktree.yml"
```
