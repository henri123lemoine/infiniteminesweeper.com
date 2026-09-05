# Large-world performance

The September 2026 measurements use an August 9 local snapshot containing
12,633 revealed chunks, 37,001,574 revealed cells, 6,617,215 flags, and 6,140
players. The snapshot remains private and is not included in the repository.
The baseline is commit `539fcc0`. All measurements ran locally on an Apple M5
Max with Go 1.25.6 and `GOMAXPROCS=1`; these are not Fly capacity guarantees.

## Deployment constraints

`fly.toml` specifies one `shared-cpu-1x`, 512 MB RAM, region `ewr`, and a
persistent `/data` volume. The most recent successful GitHub deployment,
[August 14, 2026](https://github.com/henri123lemoine/infiniteminesweeper.com/actions/runs/31781142976),
deployed the baseline to machine `0802e5da140e98`. Direct Fly inspection was
blocked by an unauthenticated local CLI, so current CPU credits, RSS, disk usage,
volume capacity, and traffic have not been verified.

Fly's [shared CPU baseline](https://fly.io/docs/machines/cpu-performance/) is
6.25% per shared vCPU after burst credits are exhausted. A shared CPU is not a
full sustained core. The configured machine's volume has a documented
[4,000 IOPS / 16 MiB/s limit](https://fly.io/docs/volumes/overview/).

The app keeps its authoritative world in one process. Adding machines would
split players across independent worlds; horizontal scaling needs an explicit
shared-state design. `GOMAXPROCS=1` also intentionally restricts Go execution to
one core. If live metrics show sustained CPU throttling after these fixes,
evaluate a performance CPU before increasing player capacity.

`GOMEMLIMIT=400MiB` leaves runtime headroom in the configured 512 MB machine.
It is a [soft limit](https://go.dev/doc/gc-guide#Memory_limit), not an RSS cap or
protection against an arbitrarily large live world. Revisit it when resizing.

## Changes

- Keep up to 4,096 chunk subscriptions per player. The largest accepted view
  plus prefetch is 56 by 56 chunks; its four-chunk retention margin fits inside
  64 by 64. The old 1,536 cap evicted visible chunks, causing repeated snapshots
  even when the viewport did not move and leaving some visible cells unsubscribed.
- Grow each serialized flag group's cell slice according to that group's size.
  Reserving the entire chunk's flag count for every group multiplied memory by
  the number of different flags in the chunk.
- Share immutable flag slices with snapshots. Copy a chunk's flags on its next
  mutation, including WAL replay, while other mutable snapshot state is copied
  at capture time. The persisted format is unchanged.
- Keep overview levels 1 through 16 warm. Cache levels 32 and 64 together in a
  1,024-entry LRU, with at most 5 MiB of pixel data, invalidated on mutations.
  Iterate revealed bits and reuse the mine bitmap when rendering tiles.
- Build and compress overview patches once per matching subscription region and
  resolution. Recipients share immutable wire buffers; snapshot catch-up still
  respects subscription readiness.

## Results

| Measurement | Baseline | Optimized |
| --- | ---: | ---: |
| Live heap after loading snapshot and GC | 100.4 MiB | 88.1 MiB |
| Allocations per snapshot capture | 62.4 MiB | 9.46 MiB |
| Snapshot capture time, median of 3 | 8.48 ms | 2.73 ms |
| Snapshot load time, median of 3 | 1.80 s | 1.15 s |
| One overview update, 100 matching viewers | 238 microseconds | 49.8 microseconds |
| One overview update, 500 matching viewers | 964 microseconds | 78.3 microseconds |
| Heap sample with 100 simulated clients | 1,300 MiB | 211 MiB |
| CPU samples in a 20-second load profile | 18.31 CPU-seconds | 5.81 CPU-seconds |
| Reveal acknowledgement delay in the load probe | 592 ms | 4 ms |

The load scenario uses `tools/stress` with 100 clients, each requesting a
32-by-32-chunk viewport, panning, attempting reveals, and periodically opening
a minimap. This deliberately exercises large views. Heap is sampled five
seconds after launch, not a measured peak or RSS. CPU profiling follows for
20 seconds. The optimized server uses the proposed `GOMEMLIMIT`; the baseline
uses its existing configuration. A single additional gameplay probe measures
acknowledgement timing; it is not a latency percentile or a throughput test of
successful new reveals. Do not extrapolate these numbers directly to production.

## Reproduction and validation

Use a disposable copy of the snapshot for server runs; startup, gameplay, and
shutdown can write WAL and snapshots. `TestWorldMemory` only reads its snapshot.

```sh
IMS_BENCH_DATA_DIR=/path/to/snapshot-directory go test ./backend -run TestWorldMemory -v -count=3
go test ./backend -run '^$' -bench 'BenchmarkOverviewFanout|BenchmarkFlagSnapshot|BenchmarkOverviewTile' -benchmem -count=3
go test -tags=integration ./...
go test -race -tags=integration ./backend -run 'TestLargestView|TestChunkRegionFlag|TestSnapshotFlagsStay|TestOverview|TestFullTileMatches'
node frontend/tests/run-frontend-tests.mjs
node frontend/tests/unit-helpers.mjs
```

For the network load scenario, start the server using that disposable directory
and explicit local ports, then run `go run ./tools/stress -n 100 -url
ws://localhost:18088/ws`. Capture a 20-second profile from the optional
localhost-only `PPROF_PORT` endpoint. Run `frontend/tests/probe-latency.mjs` with
`REPO` set to the repository path and `WSURL` explicitly set to the local server.
That probe defaults to production if `WSURL` is omitted.

Run `frontend/tests/benchmark-minimap.mjs` and
`frontend/tests/benchmark-minimap-continuity.mjs` with `--url` pointing to the
local server. Both launch headless Chromium. The final browser check covers
rapid and slow zooming, panning, correct overview resolutions, payload limits,
and return-to-view behavior. Perform a manual gameplay check with `make go-run`
before merging.
