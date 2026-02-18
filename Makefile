export CGO_CFLAGS = -w

NIK_HOME ?= workspace

.PHONY: lint
lint:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: coverage
coverage: lint
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: run
run:
	go run ./cmd/nik/main.go --home $(NIK_HOME)

.PHONY: run-loop
run-loop:
	trap '' INT; \
	while true; do \
		make lint; \
		make run; \
		echo "restarting..."; \
		sleep 1; \
	done

.PHONY: run-replay
run-replay: clean
	go run ./cmd/nik/main.go --home $(NIK_HOME) -wapp-replay-history wapp_history.pb64

.PHONY: schema-diff
schema-diff:
	@go run ./tools/schemadiff -db $(NIK_HOME)/nik.db

.PHONY: db-check
db-check:
	@go run ./tools/dbcheck -db $(NIK_HOME)/nik.db

.PHONY: wapp-history-dump
wapp-history-dump:
	@go run ./tools/wapp-history-dump $(ARGS)

.PHONY: findmsg
findmsg:
	@go run ./tools/findmsg --home $(NIK_HOME)

.PHONY: call
call:
	@cd $(NIK_HOME) && go run ../tools/call $(ARGS)
