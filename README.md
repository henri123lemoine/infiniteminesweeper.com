# Infinite Minesweeper

A real-time multiplayer infinite minesweeper game built with Go backend and React frontend. Players explore an unbounded world together, competing on a global leaderboard.

## 🎮 Game Features

- **Infinite Board**: Explore endlessly in all directions (±X, ±Y)
- **Real-time Multiplayer**: See other players' reveals instantly
- **Chunk-based World**: Efficient 64×64 cell chunks with deterministic bomb placement
- **Global Leaderboard**: Compete for most cells revealed
- **Single-Core Optimized**: Designed for high performance on limited hardware

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or higher
- Modern web browser

### Running the Server

```bash
# Clone and navigate to project
git clone <repository-url>
cd infinite-minesweeper

# Install dependencies
go mod tidy

# Start the server
go run main.go
```

The server will start on `http://localhost:8001`

### Playing the Game

1. Open `http://localhost:8001` in your browser
2. Click cells to reveal them - first player to reveal gets the point
3. Numbers show count of adjacent bombs in 3×3 neighborhood
4. Compete on the leaderboard for most reveals
5. Open multiple browser tabs to test multiplayer functionality

## 🔧 Technical Architecture

### Server Design

- **Single Process**: All state in memory with periodic disk snapshots
- **Single Core**: `GOMAXPROCS(1)` for predictable performance
- **Chunk-based Storage**: 64×64 cell chunks with bitset storage (512 bytes each)
- **First-Writer Wins**: Server timestamp determines reveal ownership
- **Fan-out Broadcasting**: Reveals broadcast to 3×3 chunk neighborhood

### Client Design

- **React Frontend**: Component-based UI with hooks
- **WebSocket Communication**: Real-time bidirectional messaging
- **Optimistic Updates**: Local state with server reconciliation
- **Viewport Management**: Efficient rendering of visible chunks only

## Testing & Development

Running Tests

```bash
go test -v -race ./...
```

Running Benchmarks

```bash
go test -run=Bench -bench=. -v
```
