# Installation

## Pre-built binaries

Pre-built binaries for Linux, macOS, and Windows (amd64 and arm64) are published with each release.

Download the latest release from the [releases page](https://github.com/tinmancoding/tasktree/releases), extract the archive, and place the `tasktree` binary somewhere on your `$PATH`.

```bash
# Example: macOS arm64
curl -L https://github.com/tinmancoding/tasktree/releases/latest/download/tasktree_Darwin_arm64.tar.gz | tar xz
sudo mv tasktree /usr/local/bin/
```

## Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/tinmancoding/tasktree.git
cd tasktree
go build -o tasktree ./cmd/tasktree
```

## Verify

```bash
tasktree --help
```

## Shell completion

Tasktree uses [Cobra](https://github.com/spf13/cobra) and supports completion for bash, zsh, fish, and PowerShell.

```bash
# zsh
tasktree completion zsh > "${fpath[1]}/_tasktree"

# bash
tasktree completion bash > /etc/bash_completion.d/tasktree

# fish
tasktree completion fish > ~/.config/fish/completions/tasktree.fish
```
