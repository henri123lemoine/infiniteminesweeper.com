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

```bash
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

- Key events: connections, reveals, rate limit hits, errors
- Performance metrics: reveal latency, memory usage, active connections

## A few additional notes

- The content of cells should never be stored anywhere (determined from cell coordinates with SplitMix64). Nor seeds (determined from chunk coordinates).
- The server stores the world state (as in, what cells are revealed) in a bitset, as well as the leaderboard.

# OTHER TODOS

- [ ] Flow reveals should be sent in batches, not one by one. E.g.: user encounters a large flow, sends this full area (which may be incompassing multiple chunks) in a message to the server; the server checks that this is a single valid flow, and if so, gives score, counts this as 1 click for purposes of rate limiting, and broadcasts all the reveals to the 3x3 neighborhood, again in a single message.
- [ ] Stop people from joining the game with a different id but the same exact username as someone else
- [x] Better mobile support; e.g. u gotta reload when joining on mobile for some reason or else it disconnects or smth?
- [x] Self score at the top
- [x] Better scoring system
- [ ] Server stores last known player location (x,y) and sends it to the client on join, as well as subscriptions to adjacent chunks. That way, instead of having to call hello, then call subscribe on each chunk in a 3x3 grid, you just call hello and the server deals with connecting you to what you need. Actually, maybe frontend also stores the offset to the center of the chunk you're in, so that all that the server need to store is the chunk you're in.
- [ ] Minimap like in old game version
- [ ] Implement the WAL sketched in the plan (`replay.log`) and truncate it when a snapshot succeeds
- [ ] Origin check + SameSite cookies for CSRF
- [ ] Per‑IP or per‑/24 subnet connection caps
- [ ] Expose `/admin/debug/pprof` behind basic‑auth on Fly ≈ free profiling
- [ ] Lazy‑load React via `defer`/`async` + bundle with esbuild for ~70% initial JS size
- [ ] Use `devicePixelRatio` when sizing the `<canvas>` to avoid blurry tiles on mobile
- [ ] Persist `viewX/viewY` in `sessionStorage` so refreshes don’t reset the camera
- [ ] When the server finds itself being basically empty, it should check the latest snapshot for validation. In fact, the server should occasionally check this
- [ ] Cells should be 32x32 pixels.

- [x] Flags (can be client side only for now, as in you can right click to flag but it doesn't do anything concrete other than show a flag icon)
- [x] Color wheel for user flags (on first join)
  - [ ] Still needs a minor fix (misalignment between color wheel and selected color)
- [x] Chords (client-side)
- [ ] Cleaner looking flags
- [x] Server-side flags for scoring system, remove undoing option

- [ ] Add message compression with zstd
- [ ] Implement per-player token bucket for seed requests (200/min)
- [ ] Add reveal rate limiting protection
- [ ] Create soft-ban mechanism for suspicious activity
- [ ] Add rate limit violation logging
- [ ] Add HTTP endpoint for leaderboard queries (/leaderboard)
- [ ] Implement zoom controls
- [ ] Add user flag appearance to leaderboard display component
- [ ] Add connection status and player count indicators
- [ ] Usernames max 20 characters (I hate fun)
- [ ] Way to see user statistics (e.g. how many reveals, how many flags, how many exploded bombs, points over time, etc.)
- [ ] Client should occasionally hash local chunk reveals and send that to server for validation that there is match. If there is a mismatch, the client is unsubscribe and must resubscribe to the chunk.

## Game Fun

- [ ] Large-scale variance in bomb density. Some regions, which are rare but which you can reasonably gradient ascent towards, have a much higher density of bombs than others, and receive accordingly high point multipliers; while some regions are quite safe, with weaker players organically gravitating towards them (on the virtue of being less capable of solving the higher density regions).
- [ ] More points for higher density of active players in a region. A button to pay coins to shoot yourself to this location of globally highest player density.
- [ ] Reveals are only allowed for cells that are two-adjacent to a revealed cell. This encourages the revealed map into a

## Math

In theory, a chunk should contain:

- 8 bytes per line x 64 lines = 512 bytes per chunk
- Add to that flags

I think we might be able to handle more than a 3x3 grid of chunks per player. Something as large as 7x7 might make sense. Hmm, this could permit a minimap of variating sizes, possible to toggle between 3x3 and 5x5 and 7x7 chunks.

Speaking of the minimap. It's basically an image. It's built and rendered entirely from the client. So we're basically just turning e.g. (3 chunks x 64 cells)^2 = 192x192 into a binary black-and-white image. Oh wait, maybe not binary; we chould have pixels that are the color of flags, or black for bomb, slightly red for revealed 1, slightly green for revealed 2, etc. That could be quite nice. One of the most important things though for this game to work is to get the strong feeling that the map is teeming with opponents, so we really need a live update of this minimap as we move around. Worried this is going to be expensive. Might need to look into video streaming, because this is basically what we're going to be doing.
