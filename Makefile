INSTALL_DIR := $(HOME)/.local/bin
BINARY := tt2

.PHONY: test run-help install

test:
	go test ./...

run-help:
	go run ./cmd/tt2 --help

install:
	@mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/tt2
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
