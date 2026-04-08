# Tasktree

**Tasktree** is a CLI tool for managing task-focused multi-repository workspaces.

When a feature, bugfix, or investigation spans multiple repositories, tasktree helps you collect them into a single directory, check out the right branches, and share the setup with teammates or CI — via a single shareable file: `Tasktree.yml`.

```bash
tasktree init
tasktree add git@github.com:myorg/api.git --branch feature/payments
tasktree add git@github.com:myorg/web.git --branch feature/payments
# → Tasktree.yml written. Share it with your team.

# Teammate — place Tasktree.yml in an empty directory, then:
tasktree apply
# → Both repos cloned at the right branch.
```

## Get started

- [Installation](getting-started/installation.md)
- [Quickstart](getting-started/quickstart.md)

## Learn more

- [Concepts: What is a tasktree?](concepts/overview.md)
- [CLI Reference](cli/overview.md)
- [Common Patterns](patterns/multi-service-feature.md)
