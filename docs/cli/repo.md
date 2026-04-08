# tasktree repo

Manage repository aliases.

## Synopsis

```
tasktree repo <subcommand>
```

## Description

The `repo` command is a parent for subcommands that manage the global repository alias catalog at `~/.config/tasktree/repos.yml`. Aliases allow you to use short names instead of full clone URLs with `tasktree add`.

See [Repository Aliases](../concepts/repo-aliases.md) for a conceptual overview.

## Subcommands

### repo add-alias

```
tasktree repo add-alias <alias> <clone-url>
```

Register a new alias manually.

```bash
tasktree repo add-alias payments git@github.com:myorg/payments-service.git
# Added alias payments -> git@github.com:myorg/payments-service.git
```

**Errors:**

| Error | Cause |
|---|---|
| `alias <alias> is already in use by <url>` | The alias already points to a different URL. |

---

### repo remove-alias

```
tasktree repo remove-alias <alias>
```

Remove an existing alias.

```bash
tasktree repo remove-alias payments
# Removed alias payments
```

**Errors:**

| Error | Cause |
|---|---|
| `alias not found: <alias>` | The alias does not exist in the catalog. |

---

### repo aliases

```
tasktree repo aliases
```

List all registered aliases.

```bash
tasktree repo aliases
```

```
ALIAS           URL
api             git@github.com:myorg/api.git
myorg/api       git@github.com:myorg/api.git
payments        git@github.com:myorg/payments-service.git
```
