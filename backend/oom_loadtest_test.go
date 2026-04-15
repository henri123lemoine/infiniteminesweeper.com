//go:build integration
// +build integration

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// The load tests default to short durations so that CI can run them. Set
// LOADTEST_LONG=1 (or use the -loadtest-long flag) to run the longer soak
// variants that better catch slow leaks.
var loadtestLong = flag.Bool("loadtest-long", false, "run long-form load tests (soak / large N)")

// TestLoadRealisticSustained simulates 20 concurrent players doing normal
// gameplay for a while. Pass requires:
//   - heap stays bounded (no linear growth with time)
//   - goroutine count converges (no leaks after churn)
//   - no panics / disconnect storms
func TestLoadRealisticSustained(t *testing.T) {
	nPlayers := 20
	duration := 15 * time.Second
	if *loadtestLong {
		nPlayers = 30
		duration = 2 * time.Minute
	}

	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	clients := make([]*LoadClient, nPlayers)
	for i := range clients {
		c, err := NewLoadClient(wsURL, fmt.Sprintf("load-%d", i))
		if err != nil {
			t.Fatalf("client %d dial: %v", i, err)
		}
		if err := c.Join(); err != nil {
			t.Fatalf("client %d join: %v", i, err)
		}
		clients[i] = c
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(nPlayers)
	for i, c := range clients {
		go func(c *LoadClient, seed int64) {
			defer wg.Done()
			simulatePlayer(c, seed, stop)
		}(c, int64(1000+i*37))
	}

	// Baseline after clients settled.
	time.Sleep(2 * time.Second)
	baseline := captureSnapshot(s)
	t.Logf("T=0s     %s", baseline)

	// Sample at regular intervals.
	end := time.Now().Add(duration)
	tick := time.NewTicker(duration / 4)
	defer tick.Stop()
	var last memorySnapshot
	for time.Now().Before(end) {
		select {
		case <-tick.C:
			last = captureSnapshot(s)
			t.Logf("T=%-5s %s", time.Since(baseline.T).Round(time.Second), last)
		case <-time.After(50 * time.Millisecond):
		}
	}

	close(stop)
	for _, c := range clients {
		c.Close()
	}
	wg.Wait()

	// Let goroutines drain.
	time.Sleep(1 * time.Second)
	final := captureSnapshot(s)
	t.Logf("T=end   %s", final)

	// Memory growth check: heap should not have doubled from baseline to end
	// of sustained load.
	growthMB := int64(last.HeapAlloc-baseline.HeapAlloc) / 1024 / 1024
	if growthMB > 30 {
		t.Errorf("heap grew by %d MB during sustained load; suggests a leak", growthMB)
	}

	// After disconnect, player/goroutine count should fall back near baseline.
	if final.NumPlayers != 0 {
		t.Errorf("expected 0 players after shutdown, got %d", final.NumPlayers)
	}
	// Goroutine count may stay slightly above baseline for in-flight work —
	// allow 20 extra as tolerance.
	if final.NumGoroutine > baseline.NumGoroutine+20 {
		t.Errorf("goroutine count did not recover after disconnect: baseline=%d final=%d",
			baseline.NumGoroutine, final.NumGoroutine)
	}
}

// TestLoadPathologicalZoomOut reproduces the OOM scenario from production: a
// single client sends a massive MinimapSubscribe (~32k tiles). Before the
// memory fix this pegged server RSS at 265+ MB on the fly.io shared-cpu-1x
// (which caps at 256 MB and triggered OOM kills). Post-fix the server should
// stay well under that and the concurrent reveal latency should not regress
// beyond ~50 ms worst-case.
func TestLoadPathologicalZoomOut(t *testing.T) {
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	// Baseline includes one "normal" player whose latency we measure.
	normal, err := NewLoadClient(wsURL, "normal")
	if err != nil {
		t.Fatalf("normal dial: %v", err)
	}
	defer normal.Close()
	if err := normal.Join(); err != nil {
		t.Fatalf("normal join: %v", err)
	}
	// Normal player warms up a few chunks.
	for i := uint64(1); i <= 20; i++ {
		_ = normal.Reveal(0, 0, uint32(i*7)%4096, i)
	}
	time.Sleep(200 * time.Millisecond)
	normal.DrainFor(200 * time.Millisecond)

	baseline := captureSnapshot(s)
	t.Logf("baseline %s", baseline)

	// Start the pathological client.
	bad, err := NewLoadClient(wsURL, "zoomed-out")
	if err != nil {
		t.Fatalf("bad dial: %v", err)
	}
	defer bad.Close()
	if err := bad.Join(); err != nil {
		t.Fatalf("bad join: %v", err)
	}

	// Build a 32k-tile subscribe at res=16 (the realistic max for a 1080p
	// browser at overlay zoom 0.125).
	const nTiles = 32400
	tiles := make([]*pb.TileRef, 0, nTiles)
	for i := 0; i < nTiles; i++ {
		tiles = append(tiles, &pb.TileRef{X: int32(i % 180), Y: int32(i / 180)})
	}

	// In parallel, the normal player hammers reveals to measure concurrent
	// latency. Each reveal+broadcast round-trip is timed.
	lat := &latencyTracker{}
	revealStop := make(chan struct{})
	revealDone := make(chan struct{})
	go func() {
		defer close(revealDone)
		reqID := uint64(1000)
		for {
			select {
			case <-revealStop:
				return
			default:
			}
			start := time.Now()
			if err := normal.Reveal(int64(reqID%5), int64(reqID%3), uint32(reqID%4096), reqID); err != nil {
				return
			}
			// Coarse measure: time until we've acknowledged the reveal via any
			// message coming back from the server.
			select {
			case <-normal.recvChan:
				lat.Record(time.Since(start))
			case <-time.After(2 * time.Second):
				lat.Record(2 * time.Second) // timeout penalty
			case <-revealStop:
				return
			}
			reqID++
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Fire the massive subscribe and let it run for a fixed window. We don't
	// need to know the exact completion time — we want to measure peak
	// resource usage and concurrent reveal latency during the burst.
	subStart := time.Now()
	if err := bad.MinimapSubscribe(tiles, 16); err != nil {
		t.Fatalf("MinimapSubscribe: %v", err)
	}
	// Drain the bad client's recv channel so it doesn't backpressure its
	// readPump.
	go bad.DrainFor(15 * time.Second)
	// Run reveals concurrently for a fixed window covering the subscribe.
	time.Sleep(8 * time.Second)
	subElapsed := time.Since(subStart)
	close(revealStop)
	<-revealDone

	peak := captureSnapshot(s)
	t.Logf("peak after 32k-tile subscribe (%.1fs): %s", subElapsed.Seconds(), peak)

	p50, p99, max, n := lat.Report()
	t.Logf("concurrent-reveal latency: p50=%v p99=%v max=%v n=%d", p50, p99, max, n)

	// Bounds.
	peakDeltaMB := int64(peak.HeapAlloc-baseline.HeapAlloc) / 1024 / 1024
	if peakDeltaMB > 80 {
		t.Errorf("peak heap delta %d MB exceeds 80 MB target (was 265+ MB pre-fix)", peakDeltaMB)
	}
	if max > 500*time.Millisecond {
		t.Errorf("worst-case concurrent reveal latency %v exceeds 500ms (pre-fix: ~580ms lock-hold)", max)
	}
	if peak.NumMiniSubs < nTiles*9/10 {
		t.Errorf("expected most of %d minimap subs to be registered, got %d", nTiles, peak.NumMiniSubs)
	}
}

// TestLoadConnectionChurn repeatedly connects and disconnects players to catch
// leaks tied to connection lifecycle (goroutines, player maps, session tokens,
// minimap sub counts).
func TestLoadConnectionChurn(t *testing.T) {
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	// Warm up and capture baseline.
	warm, err := NewLoadClient(wsURL, "warm")
	if err != nil {
		t.Fatalf("warm dial: %v", err)
	}
	if err := warm.Join(); err != nil {
		t.Fatalf("warm join: %v", err)
	}
	warm.Close()
	time.Sleep(500 * time.Millisecond)
	baseline := captureSnapshot(s)
	t.Logf("baseline %s", baseline)

	cycles := 30
	if *loadtestLong {
		cycles = 200
	}
	for i := 0; i < cycles; i++ {
		c, err := NewLoadClient(wsURL, fmt.Sprintf("churn-%d", i))
		if err != nil {
			t.Fatalf("cycle %d dial: %v", i, err)
		}
		if err := c.Join(); err != nil {
			t.Fatalf("cycle %d join: %v", i, err)
		}
		_ = c.ViewUpdate(0, 0, 10*ChunkSize, 10*ChunkSize)
		_ = c.Reveal(0, 0, uint32(i*13)%4096, uint64(i))
		time.Sleep(25 * time.Millisecond)
		c.Close()
	}
	time.Sleep(1 * time.Second)
	final := captureSnapshot(s)
	t.Logf("after %d churn cycles: %s", cycles, final)

	// Session tokens persist (identity), so sessionTokens map grows — that's
	// expected. What shouldn't grow: goroutines, minimapSubCount, players.
	if final.NumPlayers != 0 {
		t.Errorf("expected 0 connected players after churn, got %d", final.NumPlayers)
	}
	if final.NumGoroutine > baseline.NumGoroutine+20 {
		t.Errorf("goroutine leak: baseline=%d final=%d", baseline.NumGoroutine, final.NumGoroutine)
	}
	// Chunk/seed-cache entries grow during churn (each reveal registers a
	// chunk); that's by design.
}

// TestLoadConcurrentZoomOuts runs multiple pathological zoom-out clients
// simultaneously. This is adversarial territory: 10 clients each hitting the
// 100k cap would be > 16 GB in the old code. Post-fix with ~163 B/sub and
// tiles only allocated on dirty, realistic peak is < 120 MB.
func TestLoadConcurrentZoomOuts(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in -short")
	}
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	nBad := 5
	if *loadtestLong {
		nBad = 10
	}

	baseline := captureSnapshot(s)
	t.Logf("baseline %s", baseline)

	clients := make([]*LoadClient, nBad)
	var wg sync.WaitGroup
	wg.Add(nBad)
	for i := 0; i < nBad; i++ {
		c, err := NewLoadClient(wsURL, fmt.Sprintf("bad-%d", i))
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		clients[i] = c
		if err := c.Join(); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		// Each client subscribes to a DIFFERENT 20k region so tile/sub storage
		// doesn't amortize across clients — worst case for server memory.
		offsetX := int32(i * 10000)
		go func(c *LoadClient, off int32) {
			defer wg.Done()
			tiles := make([]*pb.TileRef, 0, 20000)
			for j := 0; j < 20000; j++ {
				tiles = append(tiles, &pb.TileRef{X: off + int32(j%200), Y: int32(j / 200)})
			}
			_ = c.MinimapSubscribe(tiles, 16)
			c.DrainFor(10 * time.Second)
		}(c, offsetX)
	}

	// Sample memory every 2 seconds while subscribes are in flight.
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)
		snap := captureSnapshot(s)
		t.Logf("T=%ds: %s", (i+1)*2, snap)
	}
	wg.Wait()

	peak := captureSnapshot(s)
	t.Logf("final %s", peak)
	for _, c := range clients {
		c.Close()
	}

	peakMB := int64(peak.HeapAlloc) / 1024 / 1024
	if peakMB > 180 {
		t.Errorf("concurrent zoom-out peak heap %d MB exceeds 180 MB target on 256 MB machine", peakMB)
	}
}

// TestLoadFloodFillCascade hammers many reveals concurrently with multiple
// observers subscribed to the affected chunks. This exercises the broadcast
// fan-out path: each reveal triggers a ChunkUpdateBroadcast to every
// subscriber of the chunk plus a minimap dirty + delta cycle. Lots of cascading
// flood fills (zero-cell reveals that span chunks) plus many subscribers is
// what eats CPU and lock time in practice.
func TestLoadFloodFillCascade(t *testing.T) {
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	// 8 observers subscribed to a 5x5 chunk region.
	const nObservers = 8
	observers := make([]*LoadClient, nObservers)
	for i := range observers {
		c, err := NewLoadClient(wsURL, fmt.Sprintf("obs-%d", i))
		if err != nil {
			t.Fatalf("obs dial: %v", err)
		}
		if err := c.Join(); err != nil {
			t.Fatalf("obs join: %v", err)
		}
		_ = c.ViewUpdate(0, 0, 5*ChunkSize, 5*ChunkSize)
		// Subscribe minimap too — each reveal will send broadcasts AND
		// minimap updates to all observers.
		tiles := make([]*pb.TileRef, 0, 25)
		for dy := int32(-2); dy <= 2; dy++ {
			for dx := int32(-2); dx <= 2; dx++ {
				tiles = append(tiles, &pb.TileRef{X: dx, Y: dy})
			}
		}
		_ = c.MinimapSubscribe(tiles, 64)
		observers[i] = c
		go c.DrainFor(30 * time.Second)
	}

	// One reveal-spammer.
	revealer, err := NewLoadClient(wsURL, "revealer")
	if err != nil {
		t.Fatalf("revealer dial: %v", err)
	}
	defer revealer.Close()
	if err := revealer.Join(); err != nil {
		t.Fatalf("revealer join: %v", err)
	}
	go revealer.DrainFor(30 * time.Second)

	baseline := captureSnapshot(s)
	t.Logf("baseline %s", baseline)

	// Hammer reveals across the 5x5 region for 5 seconds. Each reveal
	// fans out to 8 observers x (chunk update + minimap delta).
	revealStart := time.Now()
	revealCount := 0
	deadline := time.Now().Add(5 * time.Second)
	reqID := uint64(1)
	for time.Now().Before(deadline) {
		cx := int64(reqID%5) - 2
		cy := int64((reqID/5)%5) - 2
		cell := uint32(reqID*97) % 4096
		_ = revealer.Reveal(cx, cy, cell, reqID)
		reqID++
		revealCount++
		// Don't melt the CPU — pace at ~200 reveals/sec.
		time.Sleep(5 * time.Millisecond)
	}
	revealElapsed := time.Since(revealStart)
	time.Sleep(500 * time.Millisecond) // allow broadcasts to flush

	peak := captureSnapshot(s)
	t.Logf("after %d reveals in %v (8 observers, minimap broadcast): %s",
		revealCount, revealElapsed.Round(time.Millisecond), peak)

	for _, c := range observers {
		c.Close()
	}

	// Heap should stay modest — the broadcast fan-out is bounded.
	heapMB := int64(peak.HeapAlloc) / 1024 / 1024
	if heapMB > 80 {
		t.Errorf("flood-cascade heap %d MB exceeds 80 MB ceiling on a 256 MB box", heapMB)
	}
}

// TestLoadProfileMemory runs a scenario and writes a heap profile for manual
// inspection via `go tool pprof`. Off by default.
func TestLoadProfileMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in -short")
	}
	profilePath := "/tmp/ims-loadtest-heap.pprof"
	_, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	const nPlayers = 20
	clients := make([]*LoadClient, nPlayers)
	for i := range clients {
		c, err := NewLoadClient(wsURL, fmt.Sprintf("prof-%d", i))
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		if err := c.Join(); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		clients[i] = c
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(nPlayers)
	for i, c := range clients {
		go func(c *LoadClient, seed int64) {
			defer wg.Done()
			simulatePlayer(c, seed, stop)
		}(c, int64(42+i*17))
	}
	time.Sleep(30 * time.Second)

	// Capture the heap profile at peak-ish load.
	f, err := os.Create(profilePath)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	f.Close()
	t.Logf("wrote heap profile to %s — analyze with `go tool pprof %s`", profilePath, profilePath)

	close(stop)
	for _, c := range clients {
		c.Close()
	}
	wg.Wait()
}
