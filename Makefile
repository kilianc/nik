export CGO_CFLAGS = -w

NIK_HOME ?= workspace

.PHONY: lint
lint:
	gofmt -w .
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
	cd $(NIK_HOME) && go run ../cmd/nik/main.go

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
	cd $(NIK_HOME) && go run ../cmd/nik/main.go -wapp-replay-history wapp_history.pb64

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

.PHONY: call
call:
	@cd $(NIK_HOME) && go run ../tools/call $(ARGS)

.PHONY: sessions
sessions:
	@tmux list-sessions -F '#{session_name} (#{?pane_dead,dead,alive})' 2>/dev/null \
		| grep '^nik-' || echo "no nik sessions"

.PHONY: watch
watch:
	@if [ -z "$(S)" ]; then \
		s=$$(tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^nik-' | head -1); \
		[ -z "$$s" ] && echo "no nik sessions" && exit 1; \
	else \
		s="nik-$(S)"; \
	fi; \
	tmux attach -t "$$s" -r
