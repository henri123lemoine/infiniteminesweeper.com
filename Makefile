# Build / run Infinite Minesweeper
#   MODE={development|production} make go-run      → local Go binary
#   MODE={development|production} make docker-run  → Docker image & container

MODE          ?= development
ENVFILE        = .env.$(MODE)
SHELL          := bash

FRONTEND_SRCS  := $(shell find frontend/src -type f)
ENVFILE_PATH   := $(wildcard $(ENVFILE))
SNAPFREE       ?= 0
SNAPSHOT_FILE  ?= data/snapshot.gob.gz
WAL_FILE       ?= data/wal.log

.PHONY: help proto deps frontend-build go-build go-run docker-build docker-run deploy clean fmt lint check

help:
	@echo "Targets:"
	@echo "  go-run        - Build & run with Go (uses MODE=$(MODE))"
	@echo "  docker-run    - Build image & run container (MODE=$(MODE))"
	@echo "  deploy        - Run tests & fly deploy"
	@echo "Use MODE=production for a prod bundle; default is development."

# sprite generation ── output into src/assets so Vite bundles the files
SPRITE_OUT_DIR = frontend/src/assets

spritesheet: $(SPRITE_OUT_DIR)/spritesheet.png $(SPRITE_OUT_DIR)/spritesheet.json

$(SPRITE_OUT_DIR)/spritesheet.png $(SPRITE_OUT_DIR)/spritesheet.json: \
		frontend/assets/raw/* frontend/assets/sprites.yaml scripts/python/sprite_sheet_gen.py
	@mkdir -p $(SPRITE_OUT_DIR)
	cd scripts/python && \
		uv run sprite_sheet_gen.py ../../frontend/assets/raw/ \
			../../frontend/assets/sprites.yaml \
			../../$(SPRITE_OUT_DIR)/spritesheet.png \
			../../$(SPRITE_OUT_DIR)/spritesheet.json

# code generation & deps
proto: backend/gen/proto/messages.pb.go frontend/src/gen/messages_pb.js

backend/gen/proto/messages.pb.go: proto/messages.proto
	@echo "Generating Go protobuf stubs…"
	mkdir -p backend/gen
	protoc --go_out=backend/gen --go_opt=paths=source_relative $<

frontend/src/gen/messages_pb.js: proto/messages.proto
	@echo "Generating JS protobuf stubs…"
	mkdir -p frontend/src/gen
	npx pbjs -t static-module -w es6 -o $@ $<

deps:
	cd frontend && npm ci
	go mod tidy

# front-end bundle
frontend-build: spritesheet $(FRONTEND_SRCS) frontend/vite.config.mjs $(ENVFILE_PATH) | proto deps
	@echo "Building front-end (Vite) for $(MODE)"
	cd frontend && npm run build:$(MODE)

# back-end binary
go-build: proto frontend-build
	@echo "Building backend…"
	go build -o backend/dist/backend ./backend

go-run: go-build
	@if [ "$(SNAPFREE)" = "1" ]; then \
		echo "Removing snapshot $(SNAPSHOT_FILE) and WAL $(WAL_FILE)"; \
		rm -f "$(SNAPSHOT_FILE)" "$(WAL_FILE)"; \
	fi
	@echo "Running backend (MODE=$(MODE))"
	MODE=$(MODE) backend/dist/backend

# docker image/run
docker-build: proto frontend-build spritesheet
	docker build --pull -t infiniteminesweeper .

ENVFILE_MERGED := /tmp/.env.merged
docker-run: docker-build $(ENVFILE) .env.shared
	@echo "Merging env files…"
	cat .env.shared $(ENVFILE) > $(ENVFILE_MERGED)
	docker run --env-file $(ENVFILE_MERGED) \
	           -v $(PWD)/data:/data -p 8080:8080 infiniteminesweeper

# deploy / clean
deploy:
	go test ./...
	fly deploy

clean:
	rm -rf backend/dist
	docker rmi infiniteminesweeper 2>/dev/null || true
	docker image prune -f

# formatting & linting
fmt:
	gofmt -w backend/
	cd frontend && npm run fmt

lint:
	golangci-lint run

check: fmt lint
	@echo "✓ All checks passed"
