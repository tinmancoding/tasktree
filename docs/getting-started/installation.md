# Installation

## Requirements

- **git 2.40 or later.** tasktree runs `git merge-tree --write-tree -z`, which needs 2.40. It is developed and tested with git 2.53.
- **Linux or macOS.** tasktree uses file locks (`flock`) that Windows does not provide, so Windows is not supported. Release archives are built for Linux and macOS on amd64 and arm64 only.
- **Go 1.25 or later**, only if you build from source.

Check git:

```bash
git --version
```

tasktree runs `git` from your `PATH` and uses your normal git and SSH configuration for authentication. It never prompts: a transfer that needs a password or a host key confirmation fails instead of waiting.

## Install with Go

```bash
go install github.com/tinmancoding/tasktree/v2/cmd/tasktree@latest
```

This puts `tasktree` in `$(go env GOBIN)`, or in `$(go env GOPATH)/bin` when `GOBIN` is not set. Make sure that directory is on your `PATH`.

## Install from a checkout of the repository

```bash
git clone git@github.com:tinmancoding/tasktree.git
cd tasktree
make install
```

`make install` builds `tasktree` into `~/.local/bin` and creates two symlinks next to it, `tt` and `tt2`, that point at `tasktree`. It replaces any existing file or symlink with those names. Pass `INSTALL_DIR` to install somewhere else:

```bash
make install INSTALL_DIR=/usr/local/bin
```

`tt` and `tt2` are only shortcuts. All output and documentation use the name `tasktree`.

If you still have the V1 `tasktree` binary installed, `make install` replaces `tasktree` and `tt`. Read [Importing V1 workspaces](importing-v1.md) before you do that.

## Release archives

Each release publishes archives on the [releases page](https://github.com/tinmancoding/tasktree/releases), named `tasktree_<version>_<os>_<arch>.tar.gz` (for example `tasktree_2.0.0_darwin_arm64.tar.gz`) with a `checksums.txt` file. Unpack the archive and move `tasktree` onto your `PATH`.

## Check the install

```bash
tasktree --help
tasktree --version
```

`tasktree --version` (or `tasktree version`) prints the release version. A binary built from a source checkout prints `dev+<revision>`, and one installed with `go install` prints its module version.

## Shell completion

tasktree uses Cobra, which can generate completion scripts for bash, zsh, fish and PowerShell:

```bash
tasktree completion zsh > "${fpath[1]}/_tasktree"
tasktree completion bash > /etc/bash_completion.d/tasktree
tasktree completion fish > ~/.config/fish/completions/tasktree.fish
```

## First run

```bash
tasktree setup --author alice --machine laptop
```

`setup` creates `~/.tasktree`, which holds every task record. Until you configure a remote, that directory is the only copy. Back it up, or follow the [two machines guide](../guides/two-machines.md) to keep a copy in a git remote.

## Editor support

The repository ships JSON Schemas for task records, tasktree config and templates. See [Editor setup](../reference/files.md#editor-schemas).
