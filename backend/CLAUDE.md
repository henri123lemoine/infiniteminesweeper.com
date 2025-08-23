# Backend CLAUDE.md

Backend-specific guidance for Claude Code when working with the Go server for infiniteminesweeper.com.

## Setup & Testing

- Run frontend as described in main `CLAUDE.md` file only
- `go test -v ./...` to run go tests.

- **GOMAXPROCS(1)**: Single-threaded for deterministic behavior
- **Chunk-based**: O(1) spatial lookups via coordinate hashing
- **Bitsets**: Memory-efficient revealed cell storage (1 bit per regular cell, ~1 byte per flagged cell)
- **Subscription limits**: Max chunks per player prevents memory bloat

## Architecture Overview

Server-authoritative real-time multiplayer using WebSockets with single-mutex state protection.

### Core Modules

**main.go** - Entry point

- Embeds frontend assets (`//go:embed dist/*`)
- Starts HTTP server with WebSocket upgrade

**server.go** - Central state manager

- Single `stateMu sync.RWMutex` protects all world state
- State maps: `chunks` (revealed cells), `flags` (player flags), `scores`, `subs` (chunk subscriptions)
- Message flow: Command → Validate → Update state → Ack sender → Broadcast to subscribers
- Subscription model: Players subscribe to visible chunks, LRU eviction prevents bloat

**game.go** - Core game algorithms

- **Mine generation**: Deterministic `splitmix64` PRNG (matches frontend exactly)
- **Flood fill**: BFS for revealing empty cell regions
- **Coordinate transforms**: `worldToChunk()`, `cellIndexToXY()`
- **Validation**: All player actions validated server-side

**websocket.go** - Connection management

- Binary protobuf over WebSocket
- HMAC session tokens for authentication
- Message routing to game logic
- Graceful disconnect handling

**persistence.go** - Write-Ahead Logging

- All mutations logged before state changes
- Periodic snapshots for faster recovery
- S3 (production) or local disk storage
- Recovery: Load snapshot → Replay WAL → Rebuild state

**types.go** - Core types
