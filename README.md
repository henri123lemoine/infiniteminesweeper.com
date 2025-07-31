# infiniteminesweeper.com

A real-time multiplayer infinite minesweeper game built with Go backend and React frontend. Players explore an unbounded world together, competing on a global leaderboard.

Can be played at [infiniteminesweeper.com](https://infiniteminesweeper.com).

## Features

- **Infinite Board**: Explore endlessly in all directions (±X, ±Y)
- **Real-time Multiplayer**: See other players' reveals instantly
- **Chunk-based World**: Efficient 64×64 cell chunks with optimistic updates
- **Global Leaderboard**: Compete for highest score
- **Single-Core Optimized**: Designed for high performance on limited hardware
- **Configurable Persistence**: Robust state saved via Write-Ahead Logging (WAL) to S3 or a local volume

## Quick Start

### Development

```bash
git clone https://github.com/henri123lemoine/infiniteminesweeper.com.git
cd infiniteminesweeper.com
make run-fast
# Then go to http://localhost:8080
# You may also run the following in another terminal to run the frontend in development mode
cd frontend && npm run dev
```

#### Running Tests and Benchmarks

```bash
# Run tests
go test -v -race ./...
# Run benchmarks
go test -run=Bench -bench=. -v
```

#### Build Commands

```bash
make update    # Update frontend and proto files
make build     # Full build including Docker image
make run-fast  # Quick development cycle
```

### Production

Set required environment variables (see `.env.example`).
Use `DEV=true` for a faster development setup.

Run with Docker:

```bash
make run
```

### Deployment

Deploy to Fly.io:

```bash
fly secrets set AWS_ACCESS_KEY_ID=your_access_key
fly secrets set AWS_SECRET_ACCESS_KEY=your_secret_key
make deploy
```
