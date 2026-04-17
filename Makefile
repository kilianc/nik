export CGO_CFLAGS = -w

NIK_HOME ?= workspace
BIN_DIR ?= bin

.PHONY: build
build: build-linux-$(shell go env GOARCH)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -o $(BIN_DIR)/nik ./cmd/nik/

.PHONY: build-linux-amd64
build-linux-amd64:
	@mkdir -p $(BIN_DIR)
	@docker run --rm --platform linux/amd64 -v $(CURDIR):/src -w /src \
	  -v nik-gomod-cache:/go/pkg/mod \
	  -v nik-build-cache-amd64:/root/.cache/go-build \
	  -e CGO_ENABLED=1 -e CGO_CFLAGS=-w \
	  golang:1.25 \
	  go build -o $(BIN_DIR)/nik-linux-amd64 ./cmd/nik/

.PHONY: build-linux-arm64
build-linux-arm64:
	@mkdir -p $(BIN_DIR)
	@docker run --rm --platform linux/arm64 -v $(CURDIR):/src -w /src \
	  -v nik-gomod-cache:/go/pkg/mod \
	  -v nik-build-cache-arm64:/root/.cache/go-build \
	  -e CGO_ENABLED=1 -e CGO_CFLAGS=-w \
	  golang:1.25 \
	  go build -o $(BIN_DIR)/nik-linux-arm64 ./cmd/nik/

.PHONY: build-darwin-amd64
build-darwin-amd64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 GOARCH=amd64 go build -o $(BIN_DIR)/nik-darwin-amd64 ./cmd/nik/

.PHONY: build-darwin-arm64
build-darwin-arm64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 GOARCH=arm64 go build -o $(BIN_DIR)/nik-darwin-arm64 ./cmd/nik/

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

.PHONY: lint
lint:
	gofmt -w .
	go vet ./...
	@test ! -f $(NIK_HOME)/nik.db || go run ./tools/schemadiff -db $(NIK_HOME)/nik.db

.PHONY: test
test:
	go test ./...

.PHONY: coverage
coverage: lint
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: run
run: run-daemon

.PHONY: run-daemon
run-daemon: build
	./$(BIN_DIR)/nik daemon --home $(NIK_HOME)

.PHONY: run-install
run-install: build
	./$(BIN_DIR)/nik install --home $(NIK_HOME)

.PHONY: run-replay
run-replay: build
	./$(BIN_DIR)/nik replay --home $(NIK_HOME) $(ARGS)

.PHONY: run-tui
run-tui: build
	./$(BIN_DIR)/nik tui --home $(NIK_HOME) $(ARGS)

.PHONY: secrets
secrets: build
	./$(BIN_DIR)/nik secrets --home $(NIK_HOME) $(ARGS)

.PHONY: migrate
migrate:
	@go run ./tools/migrate -db $(NIK_HOME)/nik.db $(ARGS)

.PHONY: schema-diff
schema-diff:
	@go run ./tools/schemadiff -db $(NIK_HOME)/nik.db

.PHONY: db-check
db-check:
	@go run ./tools/dbcheck -db $(NIK_HOME)/nik.db

.PHONY: wapp-history-dump
wapp-history-dump:
	@go run ./tools/wapp-history-dump $(ARGS)

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
