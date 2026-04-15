//go:build integration
// +build integration

package main

import (
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

var loadtestLong = flag.Bool("loadtest-long", false, "run long-form load tests (soak / large N)")

// TestLoadRealisticSustained: 20 players doing realistic gameplay (view
// updates + reveals + occasional minimap opens). Asserts heap stays bounded
// and goroutines/state recover after disconnect.
func TestLoadRealisticSustained(t *testing.T) {
	nPlayers, duration := 20, 15*time.Second
	if *loadtestLong {
		nPlayers, duration = 30, 2*time.Minute
	}

	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	clients := make([]*LoadClient, nPlayers)
	for i := range clients {
		c, err := NewLoadClient(wsURL)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		if err := c.Join(fmt.Sprintf("load-%d", i)); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		clients[i] = c
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(nPlayers)
	for i, c := range clients {
		go func(c *LoadClient, off int64) {
			defer wg.Done()
			simulatePlayer(c, off, stop)
		}(c, int64(i))
	}

	time.Sleep(2 * time.Second)
	baseHeap, baseGoroutines := snapshot(t, s, "T=0")
	for elapsed := time.Duration(0); elapsed < duration; elapsed += duration / 4 {
		time.Sleep(duration / 4)
		snapshot(t, s, fmt.Sprintf("T=%v", time.Since(time.Now().Add(-elapsed-duration/4)).Round(time.Second)))
	}
	lastHeap, _ := snapshot(t, s, "T=end-of-load")

	close(stop)
	for _, c := range clients {
		c.Close()
	}
	wg.Wait()
	time.Sleep(time.Second)
	finalHeap, finalGoroutines := snapshot(t, s, "T=after-disconnect")

	if growth := lastHeap - baseHeap; growth > 30 {
		t.Errorf("heap grew %d MB during sustained load — possible leak", growth)
	}
	if finalGoroutines > baseGoroutines+20 {
		t.Errorf("goroutine leak: baseline=%d final=%d", baseGoroutines, finalGoroutines)
	}
	_ = finalHeap
}

// TestLoadPathologicalZoomOut reproduces the production OOM trigger: one
// client sends a 32k-tile MinimapSubscribe while another player hammers
// reveals. Pre-fix this OOM-killed the fly.io machine. Post-fix, peak heap
// stays under 80 MB and concurrent reveals stay under 500 ms even worst-case.
func TestLoadPathologicalZoomOut(t *testing.T) {
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	normal, err := NewLoadClient(wsURL)
	if err != nil {
		t.Fatalf("normal dial: %v", err)
	}
	defer normal.Close()
	if err := normal.Join("normal"); err != nil {
		t.Fatalf("normal join: %v", err)
	}
	for i := uint64(1); i <= 20; i++ {
		_ = normal.Reveal(0, 0, uint32(i*7)%4096, i)
	}
	time.Sleep(200 * time.Millisecond)
	normal.Drain()

	baseHeap, _ := snapshot(t, s, "baseline")

	bad, err := NewLoadClient(wsURL)
	if err != nil {
		t.Fatalf("bad dial: %v", err)
	}
	defer bad.Close()
	if err := bad.Join("zoomed-out"); err != nil {
		t.Fatalf("bad join: %v", err)
	}

	const nTiles = 32400
	tiles := make([]*pb.TileRef, nTiles)
	for i := range tiles {
		tiles[i] = &pb.TileRef{X: int32(i % 180), Y: int32(i / 180)}
	}

	revealStop := make(chan struct{})
	revealDone := make(chan struct{})
	var maxLatency time.Duration
	var latencyMu sync.Mutex
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
			if normal.Reveal(int64(reqID%5), int64(reqID%3), uint32(reqID%4096), reqID) != nil {
				return
			}
			select {
			case <-normal.recv:
			case <-time.After(2 * time.Second):
			case <-revealStop:
				return
			}
			latencyMu.Lock()
			if d := time.Since(start); d > maxLatency {
				maxLatency = d
			}
			latencyMu.Unlock()
			reqID++
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if err := bad.MinimapSubscribe(tiles, 16); err != nil {
		t.Fatalf("MinimapSubscribe: %v", err)
	}
	go func() {
		for i := 0; i < 1500; i++ {
			bad.Drain()
			time.Sleep(10 * time.Millisecond)
		}
	}()
	time.Sleep(8 * time.Second)
	close(revealStop)
	<-revealDone

	peakHeap, _ := snapshot(t, s, "peak")
	t.Logf("worst concurrent-reveal latency during 32k-tile subscribe: %v", maxLatency)

	if delta := peakHeap - baseHeap; delta > 80 {
		t.Errorf("peak heap delta %d MB exceeds 80 MB target (was 265+ MB pre-fix)", delta)
	}
	if maxLatency > 500*time.Millisecond {
		t.Errorf("worst-case reveal latency %v exceeds 500ms (pre-fix: ~580ms)", maxLatency)
	}
}

// TestLoadConnectionChurn: rapid connect/reveal/disconnect to catch
// goroutine and player-state leaks in the connection lifecycle.
func TestLoadConnectionChurn(t *testing.T) {
	s, wsURL, cleanup := startLoadTestServer(t)
	defer cleanup()

	warm, _ := NewLoadClient(wsURL)
	if err := warm.Join("warm"); err != nil {
		t.Fatalf("warm join: %v", err)
	}
	warm.Close()
	time.Sleep(500 * time.Millisecond)
	_, baseGoroutines := snapshot(t, s, "baseline")

	cycles := 30
	if *loadtestLong {
		cycles = 200
	}
	for i := 0; i < cycles; i++ {
		c, err := NewLoadClient(wsURL)
		if err != nil {
			t.Fatalf("cycle %d dial: %v", i, err)
		}
		if err := c.Join(fmt.Sprintf("churn-%d", i)); err != nil {
			t.Fatalf("cycle %d join: %v", i, err)
		}
		_ = c.ViewUpdate(0, 0, 10*ChunkSize)
		_ = c.Reveal(0, 0, uint32(i*13)%4096, uint64(i))
		time.Sleep(25 * time.Millisecond)
		c.Close()
	}
	time.Sleep(time.Second)
	_, finalGoroutines := snapshot(t, s, fmt.Sprintf("after %d churn cycles", cycles))

	if finalGoroutines > baseGoroutines+20 {
		t.Errorf("goroutine leak: baseline=%d final=%d", baseGoroutines, finalGoroutines)
	}
}

// TestLoadConcurrentZoomOuts: multiple pathological zoom-out clients
// simultaneously. Adversarial — pre-fix this would have been > 1 GB.
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

	clients := make([]*LoadClient, nBad)
	var wg sync.WaitGroup
	wg.Add(nBad)
	for i := range clients {
		c, err := NewLoadClient(wsURL)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		clients[i] = c
		if err := c.Join(fmt.Sprintf("bad-%d", i)); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		go func(c *LoadClient, off int32) {
			defer wg.Done()
			tiles := make([]*pb.TileRef, 20000)
			for j := range tiles {
				tiles[j] = &pb.TileRef{X: off + int32(j%200), Y: int32(j / 200)}
			}
			_ = c.MinimapSubscribe(tiles, 16)
			for k := 0; k < 100; k++ {
				c.Drain()
				time.Sleep(100 * time.Millisecond)
			}
		}(c, int32(i*10000))
	}

	for range 5 {
		time.Sleep(2 * time.Second)
		snapshot(t, s, "")
	}
	wg.Wait()

	peakHeap, _ := snapshot(t, s, "final")
	for _, c := range clients {
		c.Close()
	}
	if peakHeap > 180 {
		t.Errorf("concurrent zoom-out peak heap %d MB exceeds 180 MB target", peakHeap)
	}
}

// simulatePlayer is the realistic-gameplay loop for TestLoadRealisticSustained.
// Single goroutine: ticks for view-pan, reveals, occasional minimap opens.
// Just enough randomness to avoid synchronized lockstep across clients.
func simulatePlayer(c *LoadClient, seed int64, stop <-chan struct{}) {
	cx, cy := seed%40-20, (seed*17)%40-20
	reqID := uint64(1)
	_ = c.ViewUpdate(cx, cy, 32*ChunkSize)
	go func() {
		for range c.recv { // drain incoming so the channel doesn't backpressure
		}
	}()

	view := time.NewTicker(250 * time.Millisecond)
	reveal := time.NewTicker(time.Second)
	mini := time.NewTicker(20 * time.Second)
	defer view.Stop()
	defer reveal.Stop()
	defer mini.Stop()

	for {
		select {
		case <-stop:
			return
		case <-view.C:
			cx += seed % 3
			cy += (seed * 7) % 3
			_ = c.ViewUpdate(cx, cy, 32*ChunkSize)
		case <-reveal.C:
			_ = c.Reveal(cx, cy, uint32(reqID*97)%4096, reqID)
			reqID++
		case <-mini.C:
			tiles := make([]*pb.TileRef, 0, 441)
			for dy := int32(-10); dy <= 10; dy++ {
				for dx := int32(-10); dx <= 10; dx++ {
					tiles = append(tiles, &pb.TileRef{X: int32(cx) + dx, Y: int32(cy) + dy})
				}
			}
			_ = c.MinimapSubscribe(tiles, 32)
			go func(ts []*pb.TileRef) {
				time.Sleep(5 * time.Second)
				_ = c.MinimapUnsubscribe(ts)
			}(tiles)
		}
	}
}
