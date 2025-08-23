# Frontend CLAUDE.md

Frontend-specific guidance for Claude Code when working with the React-based infinite minesweeper client.

## Setup & Build

- Run frontend as described in main `CLAUDE.md` file only
- Test build: `make frontend-build` (outputs to `../backend/dist`)
- Go binary embeds frontend via `//go:embed dist/*`
- Production builds are minified; use `__DEV__` flag for dev-specific code

## Architecture Overview

React SPA with WebSocket game client, canvas rendering, and optimistic state updates.

### Core Modules

**App.jsx** - Main component orchestrating game UI

- Manages tabs (play/leaderboard/advancements) and user authentication
- Coordinates between game state, renderer, and UI components

**useGameState.js** - WebSocket client and state management

- Binary protobuf protocol over WebSocket with auto-reconnect
- Optimistic updates tracked by request ID, reconciled with server events
- Dynamic chunk subscriptions based on viewport
- Commands: Reveal, Flag, Join | Events: ChunkUpdate, ScoreUpdate

**CanvasRenderer.js** - High-performance rendering engine

- Canvas-based rendering with viewport management
- Renders only visible 64×64 chunks
- Coordinate transforms: world ↔ chunk ↔ screen
- Spritesheet-based rendering using generated assets (see `assets/sprites.yaml`)

**Minimap.jsx** - Dual-mode minimap

- HUD mode: corner overlay during gameplay
- Full-screen overlay mode with navigation

**FlagSelector.jsx** - Flag customization UI

### State Strategy

- **React Refs**: Performance-critical game data (revealedCellsRef, flaggedCellsRef, playerFlagsRef)
- **React State**: UI state (tabs, overlays, user input)
- **Server Authority**: All game state authoritative on server, client predicts and reconciles

## Generated Assets

Not in git, created during build:

- `src/gen/messages_pb.js` - Protobuf stubs from `../proto/messages.proto`
- `src/assets/spritesheet.png|json` - From `assets/raw/*` + `sprites.yaml` config
