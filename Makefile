# mdl - build, test and release tooling.  Run `make` for a list of targets.

BINARY    := mdl
PKG       := ./cmd/mdl
GO        ?= go
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
BIN_DIR   := bin
DIST      := dist
INSTALL_DIR ?= $(HOME)/.local/bin
COVER_MIN ?= 85
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
RELEASE_BRANCH ?= master
LATEST_TAG = $(shell git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)
SHA256    := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

.DEFAULT_GOAL := help
.PHONY: help build run install uninstall test test-race cover cover-html cover-check \
        fmt fmt-check vet lint tidy tidy-check check ci build-all dist tag release-patch release-minor release-major release deps clean clean-dist

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
	     /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## ---- build -----------------------------------------------------------------

build: ## Build ./bin/mdl for this machine
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

run: ## Run mdl from source: make run ARGS="list -u"
	$(GO) run -ldflags "-X main.version=$(VERSION)" $(PKG) $(ARGS)

install: build ## Install to ~/.local/bin/mdl (override INSTALL_DIR / DESTDIR)
	install -d $(DESTDIR)$(INSTALL_DIR)
	install -m 0755 $(BIN_DIR)/$(BINARY) $(DESTDIR)$(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(DESTDIR)$(INSTALL_DIR)/$(BINARY)"
	@case ":$$PATH:" in *":$(INSTALL_DIR):"*) ;; *) echo "Add it to your PATH:  export PATH=\"$(INSTALL_DIR):\$$PATH\"";; esac

uninstall: ## Remove the installed binary (and the directory if now empty)
	rm -f $(DESTDIR)$(INSTALL_DIR)/$(BINARY)
	-rmdir $(DESTDIR)$(INSTALL_DIR) 2>/dev/null

deps: build ## Install beet, yt-dlp and spotdl (via `mdl install`)
	$(BIN_DIR)/$(BINARY) install

build-all: ## Cross-compile raw binaries for every platform into ./dist/bin
	@mkdir -p $(DIST)/bin
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
	  out=$(DIST)/bin/$(BINARY)_$${os}_$${arch}$$ext; \
	  echo "building $$out"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out $(PKG) || exit 1; \
	done

dist: clean-dist ## Build release archives (.tar.gz / .zip) + checksums.txt in ./dist
	@mkdir -p $(DIST)/stage
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
	  name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
	  stage=$(DIST)/stage/$$name; mkdir -p $$stage; \
	  echo "packaging $$name"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$stage/$(BINARY)$$ext $(PKG) || exit 1; \
	  cp README.md $$stage/; \
	  if [ "$$os" = windows ]; then (cd $(DIST)/stage && zip -qr ../$$name.zip $$name); \
	  else tar -C $(DIST)/stage -czf $(DIST)/$$name.tar.gz $$name; fi; \
	done
	@rm -rf $(DIST)/stage
	@cd $(DIST) && $(SHA256) *.tar.gz *.zip > checksums.txt
	@echo "Wrote $(DIST)/ (version $(VERSION))"

## ---- quality ---------------------------------------------------------------

test: ## Run the unit tests
	$(GO) test ./...

test-race: ## Run the unit tests with the race detector
	$(GO) test -race -count=1 ./...

cover: ## Run tests and print per-function coverage
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

cover-html: cover ## Open an HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

cover-check: ## Fail if total coverage is below COVER_MIN percent (default 85)
	@$(GO) test -coverprofile=coverage.out ./... >/dev/null
	@total=$$($(GO) tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	echo "total coverage: $$total% (minimum $(COVER_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVER_MIN)" 'BEGIN { exit (t+0 >= m+0) ? 0 : 1 }'

fmt: ## Format all Go files
	gofmt -w -s .

fmt-check: ## Fail if any file needs gofmt
	@out=$$(gofmt -l -s .); \
	if [ -n "$$out" ]; then echo "needs gofmt:"; echo "$$out"; exit 1; fi

vet: ## Run go vet (also for Windows, where the TUI is stubbed)
	$(GO) vet ./...
	GOOS=windows $(GO) vet ./...

lint: ## Run golangci-lint (if installed); config in .golangci.yml
	@command -v golangci-lint >/dev/null 2>&1 \
	  && golangci-lint run ./... \
	  || echo "golangci-lint not installed; skipping (https://golangci-lint.run)"

tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

tidy-check: ## Fail if go.mod / go.sum are not tidy (commit the result of `make tidy`)
	@$(GO) mod tidy
	@git diff --exit-code -- go.mod go.sum >/dev/null 2>&1 || { echo "go.mod/go.sum are not tidy: run 'make tidy' and commit"; exit 1; }

check: fmt-check tidy-check vet test-race cover-check ## Everything CI runs on every pull request

ci: check build ## Alias used by CI: check, then build

## ---- release ---------------------------------------------------------------

tag: ## Tag and push a release: make tag TAG=v1.2.3 (CI builds and publishes it)
	@case "$(TAG)" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "usage: make tag TAG=vX.Y.Z"; exit 1;; esac
	@git diff --quiet && git diff --cached --quiet || { echo "working tree is dirty"; exit 1; }
	$(MAKE) check
	git tag -a $(TAG) -m "Release $(TAG)"
	git push origin $(TAG)

release-patch: ## Release the next patch version (v0.3.5 -> v0.3.6)
	@$(MAKE) --no-print-directory release BUMP=patch

release-minor: ## Release the next minor version (v0.3.5 -> v0.4.0)
	@$(MAKE) --no-print-directory release BUMP=minor

release-major: ## Release the next major version (v0.3.5 -> v1.0.0)
	@$(MAKE) --no-print-directory release BUMP=major

release:
	@case "$(BUMP)" in patch|minor|major) ;; *) echo "usage: make release-patch | release-minor | release-major"; exit 1;; esac
	@branch=$$(git rev-parse --abbrev-ref HEAD); \
	[ "$$branch" = "$(RELEASE_BRANCH)" ] || { echo "releases are cut from $(RELEASE_BRANCH), not $$branch"; exit 1; }
	@git fetch --quiet --tags origin
	@git diff --quiet HEAD @{u} 2>/dev/null || { echo "$(RELEASE_BRANCH) is not in sync with its upstream: pull or push first"; exit 1; }
	@prev="$(LATEST_TAG)"; prev=$${prev:-v0.0.0}; \
	next=$$(echo "$$prev" | awk -F. -v bump=$(BUMP) '{ sub(/^v/, "", $$1); \
	  if (bump == "major") { $$1++; $$2 = 0; $$3 = 0 } else if (bump == "minor") { $$2++; $$3 = 0 } else { $$3++ } \
	  printf "v%d.%d.%d", $$1, $$2, $$3 }'); \
	printf "Release %s (previous %s)? [y/N] " "$$next" "$$prev"; read ans; \
	case "$$ans" in y|Y|yes) ;; *) echo "aborted"; exit 1;; esac; \
	$(MAKE) --no-print-directory tag TAG=$$next

## ---- cleanup ---------------------------------------------------------------

clean-dist:
	rm -rf $(DIST)

clean: clean-dist ## Remove build output and coverage files
	rm -rf $(BIN_DIR) coverage.out coverage.html
