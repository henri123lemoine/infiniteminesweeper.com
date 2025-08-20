# AGENTS.md / CLAUDE.md

This file provides guidance to AI agents (including Claude Code) when working with code in this repository.

## Essential Commands

### Development

- `make go-run` - Build and run the full application locally (includes frontend build)
- `make docker-run` - Build and run in Docker container with volume mount
- `MODE=production make docker-run` - Run production build in Docker
- `SNAPFREE=1 make go-run` - Run with clean state (removes snapshot/WAL)

### Build System

- `make proto` - Generate Go and JS protobuf stubs from `proto/messages.proto`
- `make spritesheet` - Generate sprite assets (requires Python + uv)
- `make frontend-build` - Build React frontend with Vite
- `make go-build` - Build Go backend binary
- `make deps` - Install all dependencies (Go modules + npm packages)

### Testing & Deployment

- `go test ./...` - Run all tests except compression benchmarks
- `RUN_COMPRESSION_BENCH=1 go test ./...` - Include compression benchmarks
- `make deploy` - Run tests and deploy to Fly.io

## Architecture Overview

This is a real-time multiplayer infinite minesweeper game with a client-server architecture using a **Command/Event-Stream** model.

### Core Concepts

- **Chunk-based World**: 64×64 cell chunks form the infinite game board
- **Server-Authoritative**: All game state lives on the server; clients perform optimistic updates
- **Real-time Updates**: WebSocket-based communication with subscriptions to chunks
- **Persistence**: Write-Ahead Logging (WAL) to S3 or local disk with periodic snapshots
- **Authentication**: Server-issued session token model to prevent player impersonation

### Data Structures

- **Chunk**: A 64x64 grid of cells, forming the base unit of the world
- **Chunk Coordinates**: A 2D coordinate (sint64 X, sint64 Y) identifying a unique Chunk
- **Cell Index**: A uint32 value from 0 to 4095 representing a specific cell within a chunk, calculated as y * 64 + x
- **Subscription Model**: Players maintain a dynamic list of chunks they are subscribed to for real-time updates

### Backend (Go)

- **Entry Point**: `backend/main.go` - HTTP server with embedded frontend
- **Core Logic**: `backend/server.go` - Game state, WebSocket handling, authentication
- **Game Rules**: `backend/game.go` - Minesweeper logic, reveal/flag operations
- **Persistence**: `backend/persistence.go` - WAL and snapshot management
- **Protocol**: Generated from `proto/messages.proto` using Protocol Buffers

### Frontend (React + Vite)

- **Main App**: `frontend/src/App.jsx` - Game UI, viewport management, WebSocket client
- **Rendering**: `frontend/src/CanvasRenderer.js` - High-performance canvas-based rendering
- **Game State**: `frontend/src/useGameState.js` - WebSocket client and optimistic updates
- **Components**: Minimap, leaderboard, flag selector, overlays

### Key Data Flow

1. **Client Commands**: Sent via WebSocket as `Reveal` messages with optimistic local updates
2. **Server Processing**: Validates commands, updates authoritative state, writes to WAL
3. **Response Flow**: `RevealAck` to originating client + `ChunkUpdateBroadcast` to all subscribers
4. **Reconciliation**: Client merges server responses with optimistic state piece-by-piece

## Development Workflow

### Environment Setup

Create three environment files from `.env.example`:

- `.env.shared` - Common variables
- `.env.development` - Dev overrides (local paths, verbose logging)
- `.env.production` - Prod overrides (S3 credentials, etc.)

### Local Development

1. `make deps` - Install dependencies
2. `make go-run` - Start server at http://localhost:8080
3. Backend serves embedded frontend; changes require rebuild

### Protocol Changes

When modifying `proto/messages.proto`:

1. Run `make proto` to regenerate Go and JS code
2. Update client/server message handling accordingly
3. Consider backward compatibility for production

### Asset Pipeline

- Raw sprites in `frontend/assets/raw/`
- Sprite configuration in `frontend/assets/sprites.yaml`
- Generated assets in `frontend/src/assets/` (git-tracked)
- Python script generates spritesheets automatically during build

## Code Architecture Notes

### State Management

- **Server State**: Single mutex protects all world state in `server.go`
- **Client State**: React refs for performance-critical data, state for UI updates
- **Optimistic Updates**: Tracked by request ID for precise reconciliation

### Performance Optimizations

- **Single-Core Design**: `GOMAXPROCS(1)` for predictable performance
- **Chunk Subscriptions**: Dynamic loading based on viewport
- **Canvas Rendering**: Direct pixel manipulation with sprite atlases
- **Compression**: gzip + LZ4 for persistence and network

### Critical Files

- `backend/server.go:90-138` - Server initialization and core state
- `backend/game.go` - Game logic (reveal, flag, flood-fill)
- `frontend/src/useGameState.js` - WebSocket client with optimistic updates
- `frontend/src/CanvasRenderer.js` - High-performance rendering engine
- `proto/messages.proto` - Complete client-server protocol

## Testing Strategy

### Go Tests

- Unit tests for game logic, persistence, compression
- Benchmarks for performance-critical paths
- Use `go test -v ./...` for verbose output

### Manual Testing

- Multi-client behavior (multiple browser tabs)
- Network resilience (disconnect/reconnect)
- Cross-chunk operations (flood-fill across boundaries)
- Persistence recovery (restart with existing data)

## Deployment Architecture

### Fly.io Production

- Multi-stage Dockerfile builds React app + Go binary
- Volume mounted at `/data` for persistence
- AWS S3 integration for WAL/snapshots in production
- Environment secrets managed via `fly secrets`

### Docker Development

- Mounts `./data` directory for local persistence
- Environment files merged at runtime
- Consistent with production environment

## Technical Implementation Details

### Server-Side Logic Implementation

**NOTE**: Some sections may be out of date with buggy behavior. See https://github.com/henri123lemoine/infiniteminesweeper.com/issues/112

#### Connection & Authentication (handleWebSocket)

The server MUST establish and verify player identity. The client is never trusted to declare its own ID.

**Authentication Logic:**
- All connections start in SPECTATOR state
- Join message transitions from SPECTATOR to PLAYER state
- If session_token is present: Look up in sessionTokens map, assign existing playerID
- If session_token is empty/not found: Generate new playerID and sessionToken, initialize player state
- Send JoinAck message with session_token (but not internal playerID)

#### Subscription Management

Players dynamically subscribe/unsubscribe from chunks based on viewport. Server maintains chunk-to-subscribers mapping for efficient broadcasting.

#### Authoritative Command Handler (handleReveal)

Processes Reveal commands under global state mutex:

1. **Command Validation**: Check user intent vs cell state (right-click, chord, standard reveal)
2. **Process Command**: Execute game logic based on intent
3. **Generate Events**: Create state changes for affected chunks, update WAL
4. **Dispatch Responses**: Send RevealAck to originator, ChunkUpdateBroadcast to subscribers

### Client-Side Logic Implementation

#### Connection & Authentication
- Read session_token from localStorage on start
- Send Join message with token to transition from SPECTATOR to PLAYER state
- Handle JoinAck message by updating localStorage

#### Subscription Management
- Dynamic chunk subscription based on viewport
- Maintain local subscribed chunks set

#### Optimistic Command Flow (handleCellClick)
- Generate unique request_id
- Perform bounded optimistic updates (subscribed chunks only)
- Send Reveal message to server

#### Intelligent Piecewise Reconciliation (onmessage)

**RevealAck Handling:**
- Reconcile primary chunk from optimistic state
- Apply authoritative outcome, update score
- Keep request_id if other chunks still pending

**ChunkUpdateBroadcast Handling:**
- Check if part of pending optimistic action
- Undo optimistic changes, apply authoritative changes
- Clean up completed reconciliations

## Project Structure

Note that non-git-tracked files are not included in this tree. Some examples are `backend/gen/proto/messages.pb.go`, which is generated by `protoc`, or `frontend/src/assets/spritesheet.png` which is generated by `python scripts/sprite_sheet_gen.py`.

```bash
~/[redacted]/infiniteminesweeper.com (main) » bettertree
.
├── .dockerignore
├── .env.example
├── .gitattributes
├── .github
│   └── workflows
│       ├── ci.yml
│       ├── claude_code.yml
│       ├── claude_code_login.yml
│       ├── fly-deploy.yml
│       └── fly-validate.yml
├── .gitignore
├── .nvmrc
├── AGENTS.md
├── Dockerfile
├── Makefile
├── README.md
├── backend
│   ├── compression_bench_test.go
│   ├── config.go
│   ├── density.go
│   ├── dev.go
│   ├── game.go
│   ├── leaderboard.go
│   ├── leaderboard_test.go
│   ├── lock_bench_test.go
│   ├── main.go
│   ├── minimap.go
│   ├── persistence.go
│   ├── server.go
│   ├── server_test.go
│   ├── types.go
│   └── websocket.go
├── docs
│   ├── ADVANCEMENTS.md
│   └── PLAN.md
├── fly.toml
├── frontend
│   ├── assets
│   │   ├── raw
│   │   │   ├── flag0.png
│   │   │   ├── flag1.png
│   │   │   ├── flag10.png
│   │   │   ├── flag11.png
│   │   │   ├── flag12.png
│   │   │   ├── flag13.png
│   │   │   ├── flag14.png
│   │   │   ├── flag15.png
│   │   │   ├── flag16.png
│   │   │   ├── flag2.png
│   │   │   ├── flag3.png
│   │   │   ├── flag4.png
│   │   │   ├── flag5.png
│   │   │   ├── flag6.png
│   │   │   ├── flag7.png
│   │   │   ├── flag8.png
│   │   │   ├── flag9.png
│   │   │   └── mine.png
│   │   └── sprites.yaml
│   ├── index.html
│   ├── package-lock.json
│   ├── package.json
│   ├── src
│   │   ├── App.jsx
│   │   ├── CanvasRenderer.js
│   │   ├── FlagSelector.jsx
│   │   ├── MinimapHUD.jsx
│   │   ├── MinimapOverlay.jsx
│   │   ├── main.jsx
│   │   ├── styles.css
│   │   └── useGameState.js
│   └── vite.config.mjs
├── go.mod
├── go.sum
├── proto
│   └── messages.proto
└── scripts
    └── python
        ├── density.py
        ├── gen_flags.py
        ├── get_img_size.py
        ├── makeflag.sh
        ├── pixel_graft.py
        ├── pixel_resize.py
        ├── pyproject.toml
        ├── score_multiplier.py
        ├── sprite_ids.yaml
        ├── sprite_sheet_gen.py
        └── uv.lock

12 directories, 77 files
```