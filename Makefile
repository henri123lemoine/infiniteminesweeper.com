.PHONY: help update build proto docker run-fast run deploy clean all

help:
	@echo "Available targets:"
	@echo "  update    - Update frontend and proto files"
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

update:
	@echo "Updating frontend..."
	npm install
	npx prettier frontend/app.jsx --write
	@echo "Building frontend..."
	npm run build
	@echo "Updating proto files..."
	./proto/update-proto.sh
	@echo "Tidying Go modules..."
	go mod tidy
	@echo "Update complete!"

build: update
	@echo "Building Docker image..."
	docker build -t infiniteminesweeper .
	@echo "Full build complete!"

run-fast: update
	@echo "Running with Go (fast mode)..."
	go run .

run: build
	@echo "Running with Docker..."
	docker run --env-file .env -p 8080:8080 infiniteminesweeper

deploy:
	@echo "Deploying to Fly.io..."
	fly deploy

clean:
	@echo "Cleaning up Docker images..."
	docker rmi infiniteminesweeper 2>/dev/null || true
	docker image prune -f
