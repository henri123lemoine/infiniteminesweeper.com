# infiniteminesweeper.com

A real-time multiplayer infinite minesweeper game built with Go backend and React frontend. Players explore an unbounded world together, competing on a global leaderboard.

Can be played at [infiniteminesweeper.com](https://infiniteminesweeper.com).

## Features

- **Infinite Board**: Explore endlessly in all directions (±X, ±Y)
- **Real-time Multiplayer**: See other players' reveals instantly
- **Chunk-based World**: Efficient 64×64 cell chunks with optimistic updates
- **Global Leaderboard**: Compete for highest score
- **Single-Core Optimized**: Designed for high performance on limited hardware
- **S3-based Persistence**: Robust data persistence with Write-Ahead Logging (WAL)

## Quick Start

### Development

```bash
git clone https://github.com/henri123lemoine/infiniteminesweeper.com.git
cd infiniteminesweeper.com
make run-fast
# Then go to http://localhost:8080
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

Set required environment variables:

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
export S3_BUCKET_NAME=infiniteminesweeper
```

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
