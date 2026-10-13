INSTALL_DIR := $(HOME)/.local/bin
BINARY := tasktree
ALIASES := tt tt2

.PHONY: test run-help docs-cli install

test:
	go test ./...

run-help:
	go run ./cmd/tasktree --help

# Regenerates the CLI reference pages from the command definitions.
docs-cli:
	rm -rf docs/reference/cli
	go run ./cmd/tasktree gen-docs docs/reference/cli

# Builds `tasktree` and points the `tt` and `tt2` aliases at it, replacing any
# existing file or symlink with those names.
install:
	@mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/tasktree
	@for alias in $(ALIASES); do \
		rm -f $(INSTALL_DIR)/$$alias; \
		ln -s $(BINARY) $(INSTALL_DIR)/$$alias; \
	done
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY) (aliases: $(ALIASES))"
