# "One-Core, Infinite Minesweeper" – Senior-Engineer Kick-Off Notes

## 0. Product & Load Assumptions

| Aspect           | Target                                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------------------- |
| Players online   | 50 – 200 steady, spikes to 1 000                                                                              |
| Board            | Unbounded in ±X/±Y (chunked)                                                                                  |
| Latency budget   | < 100 µs server-side per click at p99                                                                         |
| Hardware         | 1 vCPU, 2 GB RAM "CPU-optimized" VM                                                                           |
| Uptime           | months; leaderboard & reveal history never reset                                                              |
| Cheat-resistance | Clients can _predict_ bombs only by burning API rate-limit; server is source of truth for "who clicked first" |

## 1. High-Level Stack

```
browser (React + WebSocket)         // optimistic UI, rollback netcode-lite
        │
        │  protobuf + zstd frames
        ▼
nginx   ──►  golang single-process server   // the interesting part
        │
basic logging  ───►  stdout/stderr
periodic saves  ───►  local disk
```

_No Redis, no DB._ Everything stays in RAM; we serialize snapshots to disk every few minutes for durability.

## 2. Server Responsibilities (authoritative)

1. **Chunk-seed service**

   - HMAC-SHA256(masterSecret, "cx:cy") – exactly as in the benchmark.
   - Per-player _token bucket_ (minutes) to throttle discovery bots.

2. **Reveal registry**

   - Stores `map[chunkID]bitset` (bitset = 64×64 = 4 096 bits = 512 B).
   - First-writer wins → assign point to `playerID`; ignore later duplicates.

3. **Leaderboard**

   - `map[playerID]uint32` (atomic increment on successful reveal).
   - On snapshot, dump as top-N list + full KV for cold storage.

4. **Fan-out**

   - Each chunk keeps a _subscriber set_ → buffered chan\<revealMsg>.
   - Broadcast to 3×3 neighborhood; non-blocking send (drops counted).

5. **Persistence**

   - Copy-on-write board mirror fed via channel; flushed to gzip'd protobuf every **2 min** or **5 k reveals**, whichever first.
   - On start-up: load latest snapshot + replay tail of WAL.

## 3. Concurrency & Single-Core Discipline

- `runtime.GOMAXPROCS(1)` at boot – no accidental multi-CPU wins.
- **One global RWMutex** protects _mutating_ state (`revealed`, `leaderboard`, `subs`).

  - Readers (seed fetch, already-revealed check) take RLock for ~50 ns.
  - Writer (first-reveal) holds WLock < 2 µs (bit-test + set).

- Per-player goroutine handles its own rate-limit counters & fan-in/out to avoid cross-player contention.

We do NOT shard locks until profiling shows ≥ 5 % of wall time inside the mutex.

## 4. Data Layout Details

### 4.1 Coordinates

```go
const ChunkSize = 64  // cells
type chunkID struct{ X, Y int32 }   // fits in 4 bytes

// Bitset: 4096 bits → [64]uint64
type chunkBits [64]uint64
```

### 4.2 World State

```go
type server struct {
    secret []byte

    // chunkID → *chunkBits  (reveals)
    chunks   map[chunkID]*chunkBits
    chunksMu sync.RWMutex

    // pid → score
    scores   map[int32]uint32
    scoresMu sync.RWMutex

    // subscriptions
    subs   map[chunkID]map[int32]chan Reveal
    subsMu sync.RWMutex
}
```

_Memory math_:
`chunks` grows only as players explore. 10 000 active chunks → ~5 MiB; fits easily.

## 5. Protocol

| Msg                           | Dir               | Payload               | Notes                          |
| ----------------------------- | ----------------- | --------------------- | ------------------------------ |
| `SeedReq {cx,cy}`             | C→S               | –                     | Rate-limited 200/min           |
| `SeedResp {seed}`             | S→C               | 32 B                  | cached client-side             |
| `RevealReq {x,y}`             | C→S               | –                     | client optimistic              |
| `RevealAck {x,y, ok, scorer}` | S→C               | –                     | `ok=false` if already revealed |
| `Broadcast {x,y, scorer}`     | S→C               | fan-out to 3×3 chunks |                                |
| `LBTop {entries[]}`           | S→C (HTTP-cached) | top-100 every 10 s    |                                |

_All protobuf, zstd-compressed, batched ≤ 200 ms._

### 5.1 WebSocket Connection Management

**Connection Lifecycle:**

- Client connects → server assigns unique `playerID` and sends current leaderboard
- Client subscribes to chunks by sending `SubscribeReq {cx,cy}` for visible chunks
- Server maintains `map[playerID]*connection` with buffered channels
- On disconnect → cleanup subscriptions, remove from leaderboard broadcasts

**Reconnection Handling:**

- Client reconnects with same `playerID` → resume subscriptions
- Server sends missed reveals for subscribed chunks (last 100 per chunk)
- Client reconciles optimistic state with server state

**State Synchronization:**

- New clients receive current leaderboard immediately
- Chunk subscriptions include current reveal state
- Clients maintain local optimistic board with rollback capability

## 6. Game Mechanics

**Infinite Board Experience:**

- Standard Minesweeper rules apply (click to reveal, numbers indicate adjacent bombs)
- Board extends infinitely in all directions (±X, ±Y)
- Players can explore anywhere, creating a shared world
- All players see each other's reveals in real-time
- Leaderboard tracks total cells revealed (not just bombs found)

**Player Interaction:**

- Click any cell to reveal it (if not already revealed)
- First player to reveal a cell gets the point
- Numbers show count of adjacent bombs in 3×3 neighborhood
- Flag placement is client-side only (no server state)
- Game continues indefinitely - no win/lose conditions

**Multiplayer Aspects:**

- Real-time visibility of other players' reveals
- Competitive leaderboard based on total reveals
- Collaborative exploration of the infinite space
- No player collision - multiple players can reveal same cell simultaneously

## 7. Cheat & Abuse Mitigation

1. **Seed mining** – limited by token bucket; at 200/min the entire 64×64 region around a player still costs 16 s wall time.
2. **Reveal sniping** – first-writer wins based on server wall clock (`time.Now().UnixNano()` monotonic).
3. **Message forgery / tamper** – WebSocket over TLS + HMAC on moves is overkill; TLS + rate-limit is fine.
4. **Automation** – heuristics: abnormal seed-request streak → soft-ban (disconnect) via in-memory blacklist.

## 8. Persistence & Recovery

- Snapshot file: `snapshot_<unix>.pb.gz` (chunks, scores, lastSeq).
- WAL (`replay.log`): append compressed `Reveal` records; truncate after snapshot sync.
- Cold storage: cron rsync to S3 every hour; keep last 100 snapshots.
- On boot:

  1. Load newest snapshot.
  2. Replay WAL.
  3. Resume accepting connections (≤ 200 ms cold start).

## 9. Monitoring & Observability

**Basic Logging:**

- Structured JSON logs to stdout/stderr
- Key events: connections, reveals, rate limit hits, errors
- Performance metrics: reveal latency, memory usage, active connections

**Health Checks:**

- `/health` endpoint returns server status
- `/metrics` endpoint for basic Prometheus metrics
- Connection count, reveal rate, memory usage

**Error Handling:**

- Graceful degradation on high load
- Circuit breaker for external dependencies
- Rate limiting with exponential backoff

## 10. Client Contract (what _must_ live client-side)

- **WorldSeed** (one 64-bit constant).
- **ChunkSize**.
- `HMAC(secret, "cx:cy")` (secret requested from server).
- `bombFromHash(splitmix64(seed, x, y))` – deterministic; identical to server helper for dispute resolution.
- Local optimistic board copy & rollback buffer (depth 50).
- Keep-alive ping every 10 s; disconnect after 30 s silence.

## A few additional notes

- The content of cells should never be stored anywhere (determined from cell coordinates with SplitMix64). Nor seeds (determined from chunk coordinates).
- The server stores the world state (as in, what cells are revealed) in a bitset, as well as the leaderboard.

# Implementation Plan

## Phase 1: Core Game Engine (Essential)

### 1. **Chunk-based Coordinate System**

- Define `chunkID` struct with X,Y coordinates
- Implement coordinate conversion between world position and chunk position
- Create chunk bitset data structure (64×64 bits = 512 bytes)
- Add helper functions for chunk boundary calculations

### 2. **Deterministic Bomb Generation**

- Implement HMAC-SHA256 seed generation: `HMAC(masterSecret, "cx:cy")`
- Create `bombFromHash()` function using splitmix64 algorithm
- Ensure identical bomb placement logic on both client and server
- Add seed caching mechanism

### 3. **Basic Server State Management**

- Set up single-core discipline with `runtime.GOMAXPROCS(1)`
- Implement global RWMutex for world state protection
- Create core data structures: `chunks`, `scores`, `subs` maps
- Add basic memory management for chunk storage

### 4. **WebSocket Connection Handler**

- Set up WebSocket server with connection management
- Implement player ID assignment system
- Create connection lifecycle management (connect/disconnect)
- Add basic message routing infrastructure

### 5. **Reveal System (Core Game Logic)**

- Implement first-writer-wins reveal mechanism
- Add atomic reveal validation (check if already revealed)
- Create reveal broadcasting to 3×3 neighborhood
- Update player scores on successful reveals

## Phase 2: Networking & Real-time Communication

### 6. **Protocol Message Handling**

- Implement protobuf message definitions for all game messages
- Add message compression with zstd
- Create message batching system (≤200ms batches)
- Handle `SeedReq/SeedResp`, `RevealReq/RevealAck`, `Broadcast` messages

### 7. **Subscription System**

- Implement chunk subscription mechanism
- Add subscription cleanup on disconnect
- Create efficient fan-out to subscribed players
- Handle subscription updates when players move

### 8. **Rate Limiting**

- Implement per-player token bucket for seed requests (200/min)
- Add reveal rate limiting protection
- Create soft-ban mechanism for suspicious activity
- Add rate limit violation logging

### 9. **Leaderboard System**

- Implement real-time leaderboard updates
- Add HTTP endpoint for leaderboard queries (/leaderboard)
- Create top-N leaderboard with caching (every 10s)
- Handle leaderboard broadcasts to connected players

## Phase 3: Client-Side Implementation

### 10. **React Frontend Setup**

- Create infinite scrolling canvas/grid component
- Implement viewport management for visible chunks
- Add mouse interaction handlers for cell reveals
- Create optimistic UI with local state management

### 11. **WebSocket Client Integration**

- Implement WebSocket connection with auto-reconnect
- Add message handling for all server message types
- Create client-side state synchronization
- Handle connection state UI feedback

### 12. **Client-Side Game Logic**

- Implement local bomb calculation for immediate feedback
- Add optimistic reveal system with rollback capability
- Create flag placement (client-side only)
- Handle reveal conflict resolution with server

### 13. **UI/UX Polish**

- Add visual feedback for reveals, numbers, bombs
- Implement smooth scrolling and zoom controls
- Create leaderboard display component
- Add connection status and player count indicators

## Phase 4: Production Readiness

### 14. **Basic Persistence**

- Implement periodic snapshot saving to disk (every 2 min or 5k reveals)
- Add snapshot loading on server startup
- Create basic state recovery mechanism
- Handle graceful shutdown with state preservation

### 15. **Health Monitoring**

- Add `/health` endpoint for server status
- Implement basic metrics collection
- Create structured logging to stdout/stderr
- Add performance monitoring (reveal latency, memory usage)

### 16. **Error Handling & Resilience**

- Implement graceful degradation under high load
- Add circuit breaker patterns for critical paths
- Create proper error boundaries in React frontend
- Handle edge cases (disconnections, malformed messages)

## Implementation Notes

- **Start with Phase 1** - get basic game working locally first
- **Test thoroughly** after each phase before moving to next
- **Focus on correctness** over performance initially
- **Single-core discipline** is crucial - don't parallelize prematurely
- **Memory efficiency** matters more than speed for this scale
- **Client-server state sync** is the most complex part - handle carefully

# OTHER TODOS

- Stop people from joining the game with a different id but the same exact username as someone else
- Better mobile support; e.g. u gotta reload when joining on mobile for some reason or else it disconnects or smth?
- Chords
- Flags (can be client side only for now, as in you can right click to flag but it doesn't do anything concrete other than show a flag icon)
- Self score at the top
- Color wheel for user flags
- Better scoring system
