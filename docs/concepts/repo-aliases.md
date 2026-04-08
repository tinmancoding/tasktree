# Repository Aliases

Repository aliases let you refer to a repo by a short name instead of its full clone URL.

## How aliases work

When you run `tasktree add`, you can pass either a full URL or an alias:

```bash
# full URL
tasktree add git@github.com:myorg/api.git

# alias (once registered)
tasktree add api
```

Aliases are resolved before any other processing. If the alias is not found, the argument is used as a URL directly.

## Automatic alias registration

When you `add` a repository, tasktree automatically registers two derived aliases in `~/.config/tasktree/repos.yml`:

1. `<repo-name>` — the bare repo name (e.g., `api`)
2. `<owner>/<repo-name>` — the owner-qualified name (e.g., `myorg/api`)

Example output from `add`:

```
Added api at api
Registered alias api -> git@github.com:myorg/api.git
Registered alias myorg/api -> git@github.com:myorg/api.git
```

If an alias already points to the same URL, it reports `already points to`. If the alias would conflict with a different URL, it is skipped:

```
Skipped alias api; already used by git@github.com:otherorg/api.git
```

## Managing aliases manually

### Add an alias

```bash
tasktree repo add-alias <alias> <clone-url>
```

```bash
tasktree repo add-alias payments git@github.com:myorg/payments-service.git
# Added alias payments -> git@github.com:myorg/payments-service.git
```

### Remove an alias

```bash
tasktree repo remove-alias <alias>
```

```bash
tasktree repo remove-alias payments
# Removed alias payments
```

### List all aliases

```bash
tasktree repo aliases
```

```
ALIAS           URL
api             git@github.com:myorg/api.git
myorg/api       git@github.com:myorg/api.git
payments        git@github.com:myorg/payments-service.git
```

## Alias file location

Aliases are stored at `~/.config/tasktree/repos.yml`:

```yaml
repos:
  - url: git@github.com:myorg/api.git
    aliases:
      - api
      - myorg/api
  - url: git@github.com:myorg/payments-service.git
    aliases:
      - payments
```

This file is global — shared across all tasktrees on the machine. You can edit it manually or manage it through the `tasktree repo` commands.
