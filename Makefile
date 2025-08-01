# Build / run Infinite Minesweeper
#   MODE={development|production} make go-run      → local Go binary
#   MODE={development|production} make docker-run  → Docker image & container

MODE          ?= development
ENVFILE        = .env.$(MODE)
SHELL          := bash
NVM_ENV        = . ~/.nvm/nvm.sh && nvm use --silent

FRONTEND_SRCS  := $(shell find frontend/src -type f)
ENVFILE_PATH   := $(wildcard $(ENVFILE))

.PHONY: help proto deps frontend-build go-build go-run docker-build docker-run deploy clean

help:
	@echo "Targets:"
	@echo "  go-run        - Build & run with Go (uses MODE=$(MODE))"
	@echo "  docker-run    - Build image & run container (MODE=$(MODE))"
	@echo "  deploy        - Run tests & fly deploy"
	@echo "Use MODE=production for a prod bundle; default is development."

# code generation & deps
proto: backend/gen/proto/messages.pb.go frontend/src/gen/messages_pb.js

backend/gen/proto/messages.pb.go: proto/messages.proto
	@echo "Generating Go protobuf stubs…"
	protoc --go_out=backend/gen --go_opt=paths=source_relative $<

frontend/src/gen/messages_pb.js: proto/messages.proto
	@echo "Generating JS protobuf stubs…"
	npx pbjs -t static-module -w es6 -o $@ $<

deps:
	cd frontend && $(NVM_ENV) && npm ci
	go mod tidy

# front-end bundle
frontend-build: $(FRONTEND_SRCS) frontend/vite.config.mjs $(ENVFILE_PATH) | proto deps
	@echo "Building front-end (Vite) for $(MODE)…"
	cd frontend && $(NVM_ENV) && npm run build:$(MODE)

# back-end binary
go-build: frontend-build
	@echo "Building backend…"
	go build -o backend/dist/backend ./backend

go-run: go-build
	@echo "Running backend (MODE=$(MODE))…"
	MODE=$(MODE) backend/dist/backend

# Docker image/run
docker-build: frontend-build proto
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
