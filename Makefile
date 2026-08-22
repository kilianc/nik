export CGO_CFLAGS = -w

NIK_HOME ?= workspace
BIN_DIR ?= bin

# VERSION is the release of record, written only by `make release`. every
# binary carries it plus the commit, so a bug report names one exact build.
VERSION := $(shell cat VERSION)
GIT_SHA := $(shell git rev-parse --short=7 HEAD 2>/dev/null || echo unknown)
PKG := github.com/kciuffolo/nik/internal/version
LDFLAGS := -X $(PKG).Number=$(VERSION) -X $(PKG).SHA=$(GIT_SHA)

# Two binaries since the split: nikd owns NIK_HOME, nikctl is what a person
# types. They install together and are always built together — a host with one
# and not the other has no working nik.
BINS := nikd nikctl

# `build` also cross-builds nikctl for linux: on a macOS host the shell
# sandbox mounts that one, since a darwin binary cannot run in the container.
.PHONY: build
build: build-linux-$(shell go env GOARCH)
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$b ./cmd/$$b/ || exit 1; \
	done

.PHONY: build-linux-amd64
build-linux-amd64:
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  docker run --rm --platform linux/amd64 -v $(CURDIR):/src -w /src \
	    -v nik-gomod-cache:/go/pkg/mod \
	    -v nik-build-cache-amd64:/root/.cache/go-build \
	    -e CGO_ENABLED=1 -e CGO_CFLAGS=-w \
	    golang:1.25 \
	    go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$b-linux-amd64 ./cmd/$$b/ || exit 1; \
	done

.PHONY: build-linux-arm64
build-linux-arm64:
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  docker run --rm --platform linux/arm64 -v $(CURDIR):/src -w /src \
	    -v nik-gomod-cache:/go/pkg/mod \
	    -v nik-build-cache-arm64:/root/.cache/go-build \
	    -e CGO_ENABLED=1 -e CGO_CFLAGS=-w \
	    golang:1.25 \
	    go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$b-linux-arm64 ./cmd/$$b/ || exit 1; \
	done

.PHONY: build-darwin-amd64
build-darwin-amd64:
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  CGO_ENABLED=1 GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$b-darwin-amd64 ./cmd/$$b/ || exit 1; \
	done

.PHONY: build-darwin-arm64
build-darwin-arm64:
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  CGO_ENABLED=1 GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$b-darwin-arm64 ./cmd/$$b/ || exit 1; \
	done

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

# what the release workflow builds: native, for whatever runner it lands on,
# stripped, and named after the platform the installer asks for.
.PHONY: build-release
build-release:
	@mkdir -p $(BIN_DIR)
	@for b in $(BINS); do \
	  CGO_ENABLED=1 go build -ldflags "-s -w $(LDFLAGS)" \
	    -o $(BIN_DIR)/$$b-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/$$b/ || exit 1; \
	done

.PHONY: lint
lint:
	gofmt -w .
	go vet ./...
	@bin/check-layering
	@test ! -f $(NIK_HOME)/nik.db || go run ./tools/schemadiff -db $(NIK_HOME)/nik.db

.PHONY: test
test:
	go test ./...

# the gate in front of a release. unlike `lint` it rewrites nothing: a check
# that fixes what it finds always passes, which is no gate at all.
.PHONY: ci
ci:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then \
	  echo "gofmt needed:"; echo "$$files"; exit 1; fi
	go vet ./...
	@bin/check-layering
	@bin/check-public
	go test ./...

# the same rule CI applies to the PR title and to every commit on the branch.
# not part of `make ci`: it needs a base to compare against, and `make ci`
# also runs on main, where the range would be empty or already landed.
.PHONY: check-commits
check-commits:
	@bin/check-commits $(ARGS)

.PHONY: coverage
coverage: lint
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: run
run: run-daemon

.PHONY: run-daemon
run-daemon: build
	./$(BIN_DIR)/nikd --home $(NIK_HOME)

.PHONY: run-install
run-install: build
	./$(BIN_DIR)/nikctl install --home $(NIK_HOME)

.PHONY: run-tui
run-tui: build
	./$(BIN_DIR)/nikctl tui --home $(NIK_HOME) $(ARGS)

.PHONY: secrets
secrets: build
	./$(BIN_DIR)/nikctl secrets --home $(NIK_HOME) $(ARGS)

.PHONY: migrate
migrate:
	@go run ./tools/migrate -db $(NIK_HOME)/nik.db $(ARGS)

.PHONY: schema-diff
schema-diff:
	@go run ./tools/schemadiff -db $(NIK_HOME)/nik.db

.PHONY: db-check
db-check:
	@go run ./tools/dbcheck -db $(NIK_HOME)/nik.db

.PHONY: timeline
timeline:
	@go run ./tools/timeline -home $(NIK_HOME) $(ARGS)

.PHONY: trigger
trigger:
	@go run ./tools/trigger -home $(NIK_HOME) $(ARGS)

.PHONY: sqlite
sqlite:
	@cd $(NIK_HOME) && CGO_ENABLED=1 go run ../tools/sqlite $(ARGS)

.PHONY: workbench
workbench:
	@cd $(NIK_HOME) && CGO_ENABLED=1 go run ../cmd/workbench $(ARGS)

.PHONY: shell-image
shell-image:
	docker build -t nik-shell:latest -f workspace/Dockerfile workspace/

# a hand-cut tag that drifts from VERSION would publish binaries stamped with
# someone else's release number. the workflow calls this before it builds.
.PHONY: check-version
check-version:
	@expected="$$(cat VERSION)"; \
	  actual="$$(echo $(tag) | sed 's|refs/tags/||')"; \
	  if [ "$$expected" != "$$actual" ]; then \
	    echo "version mismatch: VERSION=$$expected tag=$$actual"; exit 1; \
	  fi

.PHONY: release
release:
	@go run ./tools/release $(ARGS)
