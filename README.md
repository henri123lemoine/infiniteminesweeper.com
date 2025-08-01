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

```bash
git clone https://github.com/henri123lemoine/infiniteminesweeper.com.git
cd infiniteminesweeper.com
```

Then, set up the environment variables (see `.env.example`):

```bash
cp .env.example .env.shared
cp .env.example .env.development
cp .env.example .env.production
```

- `.env.shared` – variables common to all modes
- `.env.development` – dev-only stuff (local path to persistence, verbose logs, etc.)
- `.env.production` – prod-only stuff (S3 keys, etc.)

Makefile should take care of loading the correct env file based on MODE provided.

### Development

Either of these will work:

```bash
MODE=development make go-run
MODE=development make docker-run
```

Then go to http://localhost:8080

#### Running Tests and Benchmarks

```bash
# Run tests
go test -v -race ./...
# Run benchmarks
go test -run=Bench -bench=. -v
```

### Production

Run with Docker:

```bash
MODE=production make docker-run
```

### Deployment

Deploy to Fly.io:

```bash
fly secrets set AWS_ACCESS_KEY_ID=your_access_key
fly secrets set AWS_SECRET_ACCESS_KEY=your_secret_key
make deploy
```
