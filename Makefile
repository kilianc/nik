export CGO_CFLAGS = -w

NIK_HOME ?= workspace

.PHONY: build
build:
	CGO_ENABLED=1 go build -o nik ./cmd/nik/

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
run: build
	cd $(NIK_HOME) && exec ../nik

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

.PHONY: trigger
trigger:
	@go run ./tools/trigger -home $(NIK_HOME) $(ARGS)

.PHONY: call
call:
	@cd $(NIK_HOME) && go run ../tools/call $(ARGS)

.PHONY: diagnose
diagnose:
	@go run ./tools/diagnose -home $(NIK_HOME) $(ARGS)

.PHONY: replay
replay:
	@go run ./tools/replay $(ARGS)

.PHONY: shell-image
shell-image:
	docker build -t nik-shell:latest -f workspace/Dockerfile workspace/

PI ?= nik@localhost
PI_NIK_HOME ?= /home/nik

.PHONY: deploy
deploy:
	rsync -az --delete \
		--exclude workspace/ --exclude .git/ --exclude nik \
		. $(PI):/tmp/nik-deploy/
	ssh $(PI) '\
		sudo rsync -a --delete /tmp/nik-deploy/ $(PI_NIK_HOME)/git/ \
		&& sudo chown -R nik:nik $(PI_NIK_HOME)/git/ \
		&& sudo -u nik bash -c "cd ~/git && CGO_ENABLED=1 /usr/local/go/bin/go build -o nik ./cmd/nik/" \
		&& sudo systemctl restart nik \
		&& rm -rf /tmp/nik-deploy \
		&& echo "deployed $$(sudo -u nik git -C $(PI_NIK_HOME)/git rev-parse --short HEAD)"'

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
