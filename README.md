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
go mod tidy
go run .
```

#### Running Tests

```bash
go test -v -race ./...
```

Running Benchmarks

```bash
go test -run=Bench -bench=. -v
```

Generate protobuf files:

```bash
./proto/update-proto.sh
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
docker build -t infinite-minesweeper .
docker run --env-file .env -p 8080:8080 infinite-minesweeper
```

The server will start on `http://localhost:8080`

### Deployment

Deploy to Fly.io:

```bash
fly secrets set AWS_ACCESS_KEY_ID=your_access_key
fly secrets set AWS_SECRET_ACCESS_KEY=your_secret_key
fly deploy
```
