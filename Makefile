GO ?= go
BIN_DIR ?= bin
DIST_DIR ?= dist
VERSION ?= v1.0.0
COMMANDS := start-remote-shell remote-shell remote-shell-info stop-remote-shell
# For .exe names inside the windows package we append the .exe suffix at build time.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build dist test integration-test plugins-sync plugins-lint clean

build:
	@mkdir -p $(BIN_DIR)
	@for cmd in $(COMMANDS); do $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BIN_DIR)/$$cmd ./cmd/$$cmd || exit; done

dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for pair in $(PLATFORMS); do \
	  os=$${pair%%/*}; arch=$${pair#*/}; \
	  stem=remote-shell-$(VERSION)-$$os-$$arch; \
	  dir=$(DIST_DIR)/tmp/$$stem; \
	  mkdir -p $$dir; \
	  for cmd in $(COMMANDS); do \
	    if [ "$$os" = "windows" ]; then bin="$$cmd.exe"; else bin="$$cmd"; fi; \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $$dir/$$bin ./cmd/$$cmd || exit 1; \
	  done; \
	  if [ "$$os" = "windows" ]; then \
	    python3 -m zipfile -c $(DIST_DIR)/$$stem.zip $$dir; \
	  else \
	    tar -C $(DIST_DIR)/tmp -czf $(DIST_DIR)/$$stem.tar.gz $$stem; \
	  fi; \
	  rm -rf $$dir; \
	done
	@rm -rf $(DIST_DIR)/tmp
	@cd $(DIST_DIR) && sha256sum remote-shell-* | tee sha256sums.txt

test:
	$(GO) test -race ./...
	$(GO) vet ./...

integration-test: build
	python3 scripts/integration_test.py

plugins-sync:
	PLUGIN_VERSION=$(VERSION) scripts/sync-skills.sh

plugins-lint:
	scripts/lint-plugins.sh

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)