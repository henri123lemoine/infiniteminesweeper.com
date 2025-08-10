# "One-Core, Infinite Minesweeper" – Senior-Engineer Kick-Off Notes

NOTE: SOME ASPECTS OF THIS PLAN ARE OUTDATED.

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

4. **Fan-out**

   - Each chunk keeps a _subscriber set_ → buffered chan\<revealMsg>.
   - Broadcast to in-view neighborhood; non-blocking send (drops counted).

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

| Msg                           | Dir               | Payload                   | Notes                          |
| ----------------------------- | ----------------- | ------------------------- | ------------------------------ |
| `SeedReq {cx,cy}`             | C→S               | –                         | Rate-limited 200/min           |
| `SeedResp {seed}`             | S→C               | 32 B                      | cached client-side             |
| `RevealReq {x,y}`             | C→S               | –                         | client optimistic              |
| `RevealAck {x,y, ok, scorer}` | S→C               | –                         | `ok=false` if already revealed |
| `Broadcast {x,y, scorer}`     | S→C               | fan-out to in-view chunks |                                |
| `LBTop {entries[]}`           | S→C (HTTP-cached) | top-100 every 10 s        |                                |

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
- WAL (`wal.log`): append compressed `Reveal` records; truncate after snapshot sync.
- Cold storage: cron rsync to S3 every hour; keep last 100 snapshots.
- On start-up: load latest snapshot + replay tail of WAL.

## 8. Monitoring & Observability

**Basic Logging:**

- Key events: connections, reveals, rate limit hits, errors
- Performance metrics: reveal latency, memory usage, active connections
