# Make sure every recipe gets the right Node
SHELL := bash
NVM_ENV := . ~/.nvm/nvm.sh && nvm use --silent

.PHONY: help proto update build docker run-fast run deploy clean all

help:
	@echo "Available targets:"
	@echo "  update    - Update frontend and proto files"
	@echo "  proto     - Regenerate Go/JS protobuf stubs"
	@echo "  build     - Update + build Docker image (full build)"
	@echo "  docker    - Build Docker image only"
	@echo "  run-fast  - Update and run directly with Go (fast mode)"
	@echo "  run       - Full build and run with Docker"
	@echo "  deploy    - Deploy to Fly.io"
	@echo "  clean     - Clean up Docker images"
	@echo ""
	@echo "Examples:"
	@echo "  make update    # Just update frontend/proto, don't build Docker"
	@echo "  make build     # Full build including Docker image"
	@echo "  make run-fast  # Quick development cycle (no Docker)"
	@echo "  make run       # Full production-like build and run"
	@echo "  make deploy    # Deploy to production"

update: backend/dist/index.html backend/proto/messages.pb.go frontend/src/generated/messages_pb.js
	@echo "Updating frontend..."
	cd frontend && $(NVM_ENV) && npm ci
	@echo "Formatting front-end..."
	cd frontend && $(NVM_ENV) && npx prettier "src/**/*.{js,jsx,ts,tsx,css}" --write
	@echo "Tidying Go modules..."
	go mod tidy
	@echo "Update complete!"

FRONTEND_SRCS := $(shell find frontend/src -type f)
backend/dist/index.html: $(FRONTEND_SRCS) frontend/vite.config.mjs
	@echo "Building front-end (Vite)…"
	cd frontend && $(NVM_ENV) && npm run build

backend/proto/messages.pb.go: backend/proto/messages.proto
	@echo "Generating Go protobuf stubs..."
	protoc --go_out=. --go_opt=paths=source_relative $<

frontend/src/generated/messages_pb.js: backend/proto/messages.proto
	@echo "Generating JS protobuf stubs..."
	npx pbjs -t static-module -w es6 -o $@ $<

build: update
	@echo "Building Docker image..."
	docker build -t infiniteminesweeper .
	@echo "Full build complete!"

run-fast: update
	@echo "Running with Go (fast mode)..."
	go run ./backend

run: build
	@echo "Running with Docker..."
	docker run --env-file .env -v $(PWD)/data:/data -p 8080:8080 infiniteminesweeper

deploy:
	go test ./...
	@echo "Deploying to Fly.io..."
	fly deploy

clean:
	@echo "Cleaning up Docker images..."
	docker rmi infiniteminesweeper 2>/dev/null || true
	docker image prune -f
