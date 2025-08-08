# [infiniteminesweeper.com](https://infiniteminesweeper.com)

![Deploy to Fly workflow](https://github.com/henri123lemoine/infiniteminesweeper.com/actions/workflows/fly-deploy.yml/badge.svg?branch=main)

A real-time multiplayer infinite minesweeper game built with Go backend and React frontend. Players explore an unbounded world together, competing on a global leaderboard.

Can be played at [infiniteminesweeper.com](https://infiniteminesweeper.com).

## Features

- **Infinite Board**: Explore endlessly in all directions (±X, ±Y)
- **Real-time Multiplayer**: See other players' reveals instantly
- **Chunk-based World**: Efficient 64×64 cell chunks with optimistic updates
- **Global Leaderboard**: Compete for highest score
- **Single-Core Optimized**: Designed for high performance on limited hardware
- **Configurable Persistence**: Robust state saved via Write-Ahead Logging (WAL) to S3 or a local volume

## Requirements

- Go 1.23+
- Node.js 22+ and npm
- protoc (Protocol Buffers compiler)
- Protobuf JS CLI (`npx pbjs` via `protobufjs-cli`)
- Python 3.11+ and `uv` (required for local sprite generation via Makefile)

## Quick Start

```bash
git clone https://github.com/henri123lemoine/infiniteminesweeper.com.git
cd infiniteminesweeper.com
```

Create environment files. The server always loads `.env.shared` and then overlays `.env.development` or `.env.production` depending on `MODE`.

Local disk persistence (omit S3 to use DATA_DIR)

```bash
DATA_DIR=./data
```

If you use S3 in any environment, set these (plus AWS creds in your shell/secret store)

```bash
S3_BUCKET_NAME=
AWS_REGION=us-east-1
```

Copy the example environment files:

```bash
cp .env.example .env.shared
cp .env.example .env.development
cp .env.example .env.production
```

- `.env.shared` – variables common to all modes
- `.env.development` – dev-only stuff (local path to persistence, verbose logs, etc.)
- `.env.production` – prod-only stuff (S3 keys, etc.)

The backend will load these automatically on start. The Makefile passes `MODE` through to builds/runs.

### Development

Run locally:

```bash
make go-run
```

Then go to [http://localhost:8080](http://localhost:8080)

Alternatively, to run in Docker locally (serves on port 8080 and mounts `./data`):

```bash
MODE=development make docker-run
```

#### Running Tests and Benchmarks

```bash
make proto
# Run all tests except compression benchmarks
go test -v ./...
# Run compression benchmarks
RUN_COMPRESSION_BENCH=1 go test -v ./...
```

### Production

Run with Docker:

```bash
MODE=production make docker-run
```

### Deployment

Deploy to Fly.io (multi-stage Dockerfile builds the React app and a static Go binary):

```bash
fly secrets set AWS_ACCESS_KEY_ID=your_access_key
fly secrets set AWS_SECRET_ACCESS_KEY=your_secret_key
make deploy
```

### Persistence Modes

- If `S3_BUCKET_NAME` is set (and AWS creds are provided), snapshots and WAL are stored in S3. WAL is flushed periodically and truncated after successful snapshots.
- Otherwise, the server persists to `DATA_DIR` (default `./data`). In Fly, a volume is mounted at `/data` per `fly.toml`.

### Build Notes

- Vite outputs the frontend bundle to `backend/dist`, and the Go binary embeds that directory (`//go:embed dist/*`).
- Protobuf stubs are generated for both Go and JS (`make proto`).
- Sprite assets are generated into `frontend/src/assets` (`make spritesheet`). This uses Python; the Makefile calls `uv run` if you have `uv` installed. Docker builds handle this automatically.
