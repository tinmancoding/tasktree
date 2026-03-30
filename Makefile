INSTALL_DIR := $(HOME)/.local/bin
BINARY := tasktree

.PHONY: test run-help install

test:
	go test ./...

run-help:
	go run ./cmd/tasktree --help

install:
	@mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/tasktree
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
