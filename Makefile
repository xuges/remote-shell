GO ?= go
BIN_DIR ?= bin
COMMANDS := start-remote-shell remote-shell remote-shell-info stop-remote-shell

.PHONY: build test integration-test clean
build:
	@mkdir -p $(BIN_DIR)
	@for cmd in $(COMMANDS); do $(GO) build -o $(BIN_DIR)/$$cmd ./cmd/$$cmd || exit; done

test:
	$(GO) test -race ./...
	$(GO) vet ./...

integration-test:
	python3 scripts/integration_test.py

clean:
	rm -rf $(BIN_DIR)
