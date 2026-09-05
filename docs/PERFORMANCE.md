# Large-world performance

The September 2026 measurements use an August 9 local snapshot containing
12,633 revealed chunks, 37,001,574 revealed cells, 6,617,215 flags, and 6,140
players. The snapshot remains private and is not included in the repository.
The baseline is commit `539fcc0`. All measurements ran locally on an Apple M5
Max with Go 1.25.6 and `GOMAXPROCS=1`; these are not Fly capacity guarantees.

## Deployment constraints

Fly CLI inspection on September 4, 2026 confirmed one `shared-cpu-1x`,
512 MB RAM, region `ewr`, and a 1 GB persistent volume on machine
`0802e5da140e98`. Fly recorded an OOM kill at 20:30:35 PDT and an automatic
restart. During the later read-only inspection, the application had four
connections, about 281 MiB RSS, 247 MiB allocated Go heap, and 19,529 populated
chunks. The data volume was only 4% used (34 MiB). CPU credits were not measured.
No production load tests, configuration changes, or deployments were performed.

Fly's [shared CPU baseline](https://fly.io/docs/machines/cpu-performance/) is
6.25% per shared vCPU after burst credits are exhausted. A shared CPU is not a
full sustained core. The configured machine's volume has a documented
[4,000 IOPS / 16 MiB/s limit](https://fly.io/docs/volumes/overview/).

The app keeps its authoritative world in one process. Adding machines would
split players across independent worlds; horizontal scaling needs an explicit
shared-state design. `GOMAXPROCS=1` also intentionally restricts Go execution to
one core. If live metrics show sustained CPU throttling after these fixes,
evaluate a performance CPU before increasing player capacity.

`GOMEMLIMIT=350MiB` leaves runtime headroom in the configured 512 MB machine.
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
20 seconds. This earlier optimized-server run used `GOMEMLIMIT=400MiB`; the baseline
used its existing configuration. The final proposal reduces the runtime limit
to 350 MiB after inspecting the live VM, which exposes about 459 MiB to Linux. A single additional gameplay probe measures
acknowledgement timing; it is not a latency percentile or a throughput test of
successful new reveals. Do not extrapolate these numbers directly to production.

## Minimap stall and zoom limits

A fresh, private production snapshot retrieved through Fly SFTP contained
19,529 populated chunks and 30,726,954 revealed cells. Its padded extent was
495 by 3,239 chunks. A global LOD 8 image would require 102,611,520 bytes
(about 98 MiB), exceeding the 8 MiB server limit. Only the LOD 1 and 2 global
images were available. The old server silently downgraded global requests to
regional snapshots; the browser still treated the request as global and stopped
requesting missing coverage after zooming out.

The reproducible case is: join, open the minimap, wait 750 ms, then immediately
zoom all the way out. At both 1920x1080 and 3840x2160, the previous PR head
`3dac22a` remained incomplete at the 8-second timeout. The revised client
finished with verified viewport coverage in about 255 ms at 1080p; at 4K the
coarse preview already covered the target view. These are local measurements,
not network latency guarantees. The previous benchmark's LOD-only readiness
check missed this failure; the new test also verifies snapshot bounds.

The revised minimap:

- Requests bounded regions of the visible world at every zoom level. It does
  not need a dense image spanning the whole world's bounding box.
- Prefetches one small coarse preview, then requests current detail after a
  short gesture debounce. Each detail image covers the zoom range of its LOD.
  Nearby detail remains visible while newly exposed edges use the preview,
  preventing repeated jumps back to a coarse full image.
- Keeps one snapshot in flight and only the latest queued view. Request IDs
  correlate responses; a missing response triggers reconnection after 10 seconds.
  Connections retry automatically with backoff, including when an overview
  response times out. Continuous patches do not restart the detail request timer.
- Caps each requested image at 4 MiB indexed pixels and 4,096 pixels per side,
  the overview image cache at 48 MiB including pinned previews, and the display
  canvas at 8 million pixels / at most 2x device scale. Very large displays use
  a coarser LOD to respect these limits. Active/fading canvases, transient decode
  buffers, the display canvas, and browser overhead are additional memory.
- Decompresses WebSocket messages and prepares overview bitmaps in a worker.
  Transferred bitmaps are closed after copying to the cached canvas. Browsers
  without workers or worker canvas support retain the bounded synchronous path.
- Subscribes to one current detail region. Inactive cached regions are not
  marked current by patches for a different subscription.
- Admits two concurrent server snapshot builders. Regional assembly releases
  the game-state lock every 32 chunks; mutations during assembly are delivered
  in an ordered catch-up patch. Dropped overview messages close the connection
  so the client can resynchronize.

A separate test puts a populated chunk at `(10000,10000)`, making all global
images unavailable. Regional loading still completes at 1080p and 4K with
250 ms added response delay, repeated zoom reversals, and patches every 40 ms.
Client request concurrency remains one. Geometry checks cover screens through
16K; they are not browser performance measurements at those sizes.

A local network test used 100 spectator clients, three 4 MiB regional snapshots
each, and a separate small seed-request probe. Both servers loaded the fresh
world; the comparison baseline is `3dac22a`, which already contains the first
round of world-memory fixes. All 300 snapshots completed in each run:

| Measurement | Previous PR head | Revised minimap |
| --- | ---: | ---: |
| Total snapshot test time | 1.80 s | 1.87 s |
| Snapshot response p95 | 941 ms | 649 ms |
| Independent seed-request median | 438 ms | 9.4 ms |
| Independent seed-request maximum | 640 ms | 17.5 ms |
| Peak sampled Go heap | 266 MiB | 291 MiB |

The previous server used 400 MiB and the revised server 350 MiB as its Go soft
memory limit. Heap samples are not RSS peaks; the probe is a responsiveness
check, not successful-reveal throughput. The workload intentionally saturates
snapshot loading and is not a player capacity estimate for Fly's shared CPU.

The gradual zoom continuity tests at 1080p and 4K passed their visual limits: zero non-adjacent
fallback frames, maximum luminance drift below 5, p95 drift below 1, and minimum
scale-normalized SSIM above 0.65 (0.89 minimum in the final 4K run).
Dropping the first snapshot deliberately also recovered at both screen sizes,
about 10.2–10.4 seconds after the zoom gesture, without reloading the page. The final delayed 4K run recorded one 53 ms browser long task across page startup
and interaction, and some frames around 50 ms. This does not promise
perfect frame pacing on every device or unlimited capacity on a 512 MB machine.

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
local server. Both launch headless Chromium. For the production stall and
bounded request regression, use:

```sh
BASE_URL=http://localhost:18096 ASSERT_READY=1 node frontend/tests/benchmark-minimap-cold.mjs
BASE_URL=http://localhost:18096 RESPONSE_DELAY=250 CHURN=1 PATCH_TRAFFIC=1 ASSERT_READY=1 node frontend/tests/benchmark-minimap-cold.mjs
BASE_URL=http://localhost:18096 DROP_FIRST=1 ASSERT_READY=1 node frontend/tests/benchmark-minimap-cold.mjs
WSURL=ws://localhost:18096/ws METRICS_URL=http://localhost:19097/metrics node frontend/tests/benchmark-overview-load.mjs
```

The new cold/load scripts require localhost. The final browser check covers
rapid and slow zooming, panning, correct overview resolutions, payload limits,
and return-to-view behavior. Perform a manual gameplay check with `make go-run`
before merging.
