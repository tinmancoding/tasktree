# tasktree root

Print the current tasktree root path.

## Synopsis

```
tasktree root
```

## Description

Resolves and prints the absolute path of the nearest `Tasktree.yml` walking up from the current directory. Useful in shell scripts that need to reference paths relative to the tasktree root.

## Examples

```bash
cd ~/work/feature-payments/api/src
tasktree root
# /home/user/work/feature-payments
```

Use in scripts:

```bash
TASKTREE_ROOT=$(tasktree root)
echo "Working in: $TASKTREE_ROOT"
```

## Errors

| Error | Cause |
|---|---|
| `not in a tasktree` | No `Tasktree.yml` found walking up from the current directory. |
