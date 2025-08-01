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
periodic saves  ───►  S3 + local disk
```

_No Redis, no DB._ Everything stays in RAM; we serialize snapshots to S3 / local disk every few minutes for durability.

## 2. Server Responsibilities (authoritative)

1. **Chunk-seed service**

   - HMAC-SHA256(masterSecret, "cx:cy").
   - Per-player _token bucket_ (minutes) to throttle discovery bots.

2. **Reveal registry**

   - Stores `map[chunkID]bitset` (bitset = 64×64 = 4 096 bits = 512 B).
   - First-writer wins → assign point to `playerID`; ignore later duplicates.

3. **Leaderboard**

   - `map[playerID]uint32`.

4. **Fan-out**

   - Each chunk keeps a _subscriber set_ → buffered chan\<revealMsg>.
   - Broadcast to 3×3 neighborhood; non-blocking send (drops counted).

5. **Persistence**

   - Copy-on-write board mirror fed via channel; flushed to gzip'd protobuf.
   - On start-up: load latest snapshot + replay tail of WAL.

## 3. Concurrency & Single-Core Discipline

- `runtime.GOMAXPROCS(1)` at boot – no accidental multi-CPU wins.
- **One global RWMutex** protects _mutating_ state (`revealed`, `leaderboard`, `subs`).

  - Readers (seed fetch, already-revealed check) take RLock for ~100 ns.
  - Writer (first-reveal) holds WLock < 1 µs (bit-test + set).

- Per-player goroutine handles its own rate-limit counters & fan-in/out to avoid cross-player contention.

We do NOT shard locks until profiling shows ≥ 5 % of wall time inside the mutex.

## 4. Protocol

| Msg                           | Dir               | Payload               | Notes                          |
| ----------------------------- | ----------------- | --------------------- | ------------------------------ |
| `SeedReq {cx,cy}`             | C→S               | –                     | Rate-limited 200/min           |
| `SeedResp {seed}`             | S→C               | 32 B                  | cached client-side             |
| `RevealReq {x,y}`             | C→S               | –                     | client optimistic              |
| `RevealAck {x,y, ok, scorer}` | S→C               | –                     | `ok=false` if already revealed |
| `Broadcast {x,y, scorer}`     | S→C               | fan-out to 3×3 chunks |                                |
| `LBTop {entries[]}`           | S→C (HTTP-cached) | top-100 every 10 s    |                                |

_All protobuf, zstd-compressed, batched ≤ 200 ms._

### 4.1 WebSocket Connection Management

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

## 5. Game Mechanics

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

## 6. Cheat & Abuse Mitigation

1. **Seed mining** – limited by token bucket; at 200/min the entire 64×64 region around a player still costs 16 s wall time.
2. **Reveal sniping** – first-writer wins based on server wall clock (`time.Now().UnixNano()` monotonic).
3. **Message forgery / tamper** – WebSocket over TLS + HMAC on moves is overkill; TLS + rate-limit is fine.
4. **Automation** – heuristics: abnormal seed-request streak → soft-ban (disconnect) via in-memory blacklist.

## 7. Persistence & Recovery

- Snapshot file: `snapshot_<unix>.pb.gz` (chunks, scores, lastSeq).
- WAL (`replay.log`): append compressed `Reveal` records; truncate after snapshot sync.
- Cold storage: cron rsync to S3 every hour; keep last 100 snapshots.
- On boot:
  1. Load newest snapshot.
  2. Replay WAL.
  3. Resume accepting connections (≤ 200 ms cold start).

## 8. Monitoring & Observability

**Basic Logging:**

- Key events: connections, reveals, rate limit hits, errors
- Performance metrics: reveal latency, memory usage, active connections

## Implementation Notes

- Cell content is never stored (determined from cell coordinates with SplitMix64). Seeds are never stored (determined from chunk coordinates).
- Server stores world state (revealed cells) in a bitset, the flags, and maintains the leaderboard.

# TODO List

## Performance & Optimization

- [ ] Add message compression with zstd
- [ ] Flow reveals should be sent in batches, not one by one
- [ ] If a chunk is fully revealed, send condensed message instead of full chunk data

## Server Infrastructure & Security

- [ ] When server is empty, validate against latest snapshot
- [ ] Per-IP or per-/24 subnet connection caps
- [ ] Add reveal rate limiting protection
- [ ] Implement per-player token bucket for seed requests (200/min)
- [ ] Create soft-ban mechanism for suspicious activity
- [ ] Add rate limit violation logging
- [ ] Expose `/admin/debug/pprof` behind basic-auth on Fly for free profiling
- [ ] Stop people from joining with different ID but same username

## Client Features & UX

- [x] Show score gained for reveals/flags next to where it happened
  - [ ] Done, but must make it more performant, somehow, flow reveals are quite slow atm. This may be as simple as implementing the batch reveals, and setting that as a single score delta message.
- [ ] Persist `viewX/viewY` in `sessionStorage` so refreshes don't reset camera
- [ ] Server stores last known player location and auto-subscribes to adjacent chunks on join
- [x] Implement zoom controls
  - [ ] Perhaps instead of + and - zooming, zooming should be done with mouse wheel or similar.
- [ ] Add connection status and player count indicators

## Data Integrity & Validation

- [ ] Client should occasionally hash local chunk reveals and send to server for validation
- [ ] Add HTTP endpoint for leaderboard queries (/leaderboard)
- [ ] Way to see user statistics (reveals, flags, exploded bombs, points over time, etc.)
- [ ] Add user flag appearance to leaderboard display component
- [ ] Website page; rather than just direct joining the game, with a button to join, if that makes sense? Pages for leaderboard, about, user profile / stats, etc.

## Game Balance & Fun

- [ ] Rehash scoring system to make more sense (bomb = -100 always is too harsh for new players)
- [ ] Large-scale variance in bomb density with regional multipliers
- [ ] More points for higher density of active players in a region
- [ ] Reveals only allowed for cells two-adjacent to revealed cells (encourages connected exploration)

## Quality of Life

- [ ] Usernames max 20 characters
- [ ] Color wheel alignment fix for user flags

## Completed Items

- [x] Flags (client-side implementation)
- [x] Color wheel for user flags (on first join)
- [x] Server-side flags for scoring system, remove undoing option
- [x] Chords (client-side)
- [x] Self score display at the top
- [x] Better scoring system
- [x] Implement WAL and truncation on snapshot success
- [x] Cleaner looking flags
- [x] Better mobile support fixes
- [x] Minimap like in old game version
- [x] Use `devicePixelRatio` when sizing the `<canvas>` to avoid blurry tiles on mobile
- [x] Lazy-load React via `defer`/`async` + bundle with esbuild for ~70% initial JS size
- [x] Origin check + SameSite cookies for CSRF

## Miscellaneous

- Might it make sense for the client to never ever bother to unsubscribe from chunks? The server could have a queue of chunks that the client is subscribed to, max 30 or so, when the client tries to subscribe to a new chunk it drops the oldest chunk from the queue and adds the new one, and when the client disconnects, the server could just clear the queue.

## Technical Notes

**Memory Calculations:**

- 8 bytes per line × 64 lines = 512 bytes per chunk
- Possible to handle larger than 3×3 chunk grids per player (up to 7×7)
- Minimap: 3×3 chunks = (3×64)² = 192×192 binary image with color coding for flags/bombs/numbers

## Debugging nil CellCoord panic
A panic occurred in `readPump` because the server expected every Reveal/Flag message to include the new `CellCoord` field. If an outdated client uses the previous protocol version, the field is absent and the server dereferences nil. This typically happened when running `go run` without rebuilding the frontend, so browsers kept using old cached JavaScript.

To prevent mismatched clients from connecting, the handshake `Hello` message now carries a `protocol` version. The server rejects connections that do not match `ProtocolVersion`.
