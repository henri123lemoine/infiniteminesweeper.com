//go:build integration
// +build integration

package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
	"google.golang.org/protobuf/proto"
)


// Shared load-test infrastructure. Keeps the scenarios lean by centralizing
// setup, memory tracking, latency sampling, and a light-weight simulated player.

// startLoadTestServer spins up a server with the same goroutines as prod
// (leaderboard + minimap broadcaster + player position broadcaster). Disables
// the proximity rule so reveals aren't rejected in synthetic scenarios.
func startLoadTestServer(t testing.TB) (*Server, string, func()) {
	t.Helper()
	s := NewServer()
	s.proximityRadius = -1

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Background workers mimicking main.go. The minimap broadcaster is an
	// infinite-loop goroutine; we intentionally don't try to stop it between
	// sub-tests — the whole test binary exits at end-of-run, which is
	// cheaper than complicating the broadcaster with a shutdown channel
	// just for tests.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.stateMu.Lock()
				if s.lbDirty || s.lbProto == nil {
					s.buildLeaderboardUnsafe()
					s.lbDirty = false
				}
				s.stateMu.Unlock()
			}
		}
	}()
	go s.runMinimapBroadcaster()

	ts := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	cleanup := func() {
		close(stop)
		ts.Close()
	}
	return s, wsURL, cleanup
}

// LoadClient is a lightweight websocket client for load-testing. It does NOT
// use the testing.T.Fatal* family — it returns errors so scenarios can
// decide how to handle disconnections (which are legitimate in some tests).
type LoadClient struct {
	conn     *websocket.Conn
	name     string
	sendMu   sync.Mutex
	done     chan struct{}
	closed   atomic.Bool
	recvChan chan *pb.Msg
	// scratch buffers to reduce GC pressure in hot loops
	gzBuf bytes.Buffer
}

func NewLoadClient(wsURL, name string) (*LoadClient, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &LoadClient{
		conn:     conn,
		name:     name,
		done:     make(chan struct{}),
		recvChan: make(chan *pb.Msg, 1024),
	}
	go c.readLoop()
	return c, nil
}

func (c *LoadClient) readLoop() {
	defer func() {
		if !c.closed.Swap(true) {
			close(c.done)
		}
	}()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(gz)
		gz.Close()
		if err != nil {
			continue
		}
		var msg pb.Msg
		if err := proto.Unmarshal(raw, &msg); err != nil {
			continue
		}
		select {
		case c.recvChan <- &msg:
		default:
			// Drop messages the scenario isn't draining fast enough.
		}
	}
}

func (c *LoadClient) send(msg *pb.Msg) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	c.gzBuf.Reset()
	gz := gzip.NewWriter(&c.gzBuf)
	if _, err := gz.Write(data); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if c.closed.Load() {
		return io.ErrClosedPipe
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, c.gzBuf.Bytes())
}

func (c *LoadClient) Join() error {
	if err := c.send(&pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: c.name}}}); err != nil {
		return err
	}
	// Wait briefly for JoinAck — don't block forever.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-c.recvChan:
			if ack := msg.GetJoinAck(); ack != nil {
				if !ack.Ok {
					return io.EOF // treat as failed
				}
				return nil
			}
		case <-deadline:
			return io.ErrShortBuffer // timeout
		case <-c.done:
			return io.ErrClosedPipe
		}
	}
}

func (c *LoadClient) ViewUpdate(chunkX, chunkY int64, widthCells, heightCells uint32) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId:     &pb.ChunkID{X: chunkX, Y: chunkY},
		Cell:        uint32(ChunkSize*ChunkSize) / 2,
		WidthCells:  widthCells,
		HeightCells: heightCells,
	}}})
}

func (c *LoadClient) Reveal(chunkX, chunkY int64, cell uint32, requestID uint64) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
		ChunkId:   &pb.ChunkID{X: chunkX, Y: chunkY},
		Cell:      cell,
		RequestId: requestID,
	}}})
}

func (c *LoadClient) MinimapSubscribe(tiles []*pb.TileRef, resolution uint32) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_MinimapSubscribe{MinimapSubscribe: &pb.SubscribeTiles{
		Tiles:      tiles,
		Resolution: resolution,
	}}})
}

func (c *LoadClient) MinimapUnsubscribe(tiles []*pb.TileRef) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_MinimapUnsubscribe{MinimapUnsubscribe: &pb.UnsubscribeTiles{
		Tiles: tiles,
	}}})
}

// DrainFor reads and discards incoming messages for the given duration.
// Used when a scenario isn't interested in the reply stream.
func (c *LoadClient) DrainFor(d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case <-c.recvChan:
		case <-deadline:
			return
		case <-c.done:
			return
		}
	}
}

func (c *LoadClient) Close() {
	if c.closed.Swap(true) {
		return
	}
	close(c.done)
	c.conn.Close()
}

// memorySnapshot captures the set of metrics we care about for OOM diagnosis.
type memorySnapshot struct {
	T           time.Time
	HeapAlloc   uint64
	HeapInuse   uint64
	Sys         uint64
	NumGoroutine int
	NumChunks   int
	NumSubs     int
	NumMiniSubs int
	NumMiniTiles int
	NumPlayers  int
}

func captureSnapshot(s *Server) memorySnapshot {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s.stateMu.RLock()
	numChunks := len(s.chunks)
	numSubs := 0
	for _, set := range s.subs {
		numSubs += len(set)
	}
	numMiniSubs := 0
	for _, set := range s.minimapSubs {
		numMiniSubs += len(set)
	}
	numMiniTiles := len(s.minimapTiles)
	s.stateMu.RUnlock()

	s.playersMu.RLock()
	numPlayers := len(s.players)
	s.playersMu.RUnlock()

	return memorySnapshot{
		T:            time.Now(),
		HeapAlloc:    ms.HeapAlloc,
		HeapInuse:    ms.HeapInuse,
		Sys:          ms.Sys,
		NumGoroutine: runtime.NumGoroutine(),
		NumChunks:    numChunks,
		NumSubs:      numSubs,
		NumMiniSubs:  numMiniSubs,
		NumMiniTiles: numMiniTiles,
		NumPlayers:   numPlayers,
	}
}

func (m memorySnapshot) String() string {
	return fmtMB(m.HeapAlloc) + " alloc / " + fmtMB(m.HeapInuse) + " inuse / " +
		fmtMB(m.Sys) + " sys | " +
		fmtInt(m.NumGoroutine) + " goroutines, " +
		fmtInt(m.NumChunks) + " chunks, " +
		fmtInt(m.NumSubs) + " subs, " +
		fmtInt(m.NumMiniSubs) + " minisubs, " +
		fmtInt(m.NumMiniTiles) + " minitiles, " +
		fmtInt(m.NumPlayers) + " players"
}

func fmtMB(n uint64) string {
	mb := float64(n) / 1024 / 1024
	return strings.TrimRight(strings.TrimRight(
		trunc3(mb), "0"), ".") + "MB"
}

func trunc3(f float64) string {
	s := strings.Builder{}
	if f == 0 {
		return "0"
	}
	return strings.TrimSuffix(
		strings.TrimRight(
			strings.TrimRight(
				stringsBuilderFloat(&s, f),
				"0"), "."),
		".")
}

func stringsBuilderFloat(b *strings.Builder, f float64) string {
	// minimal helper; using %.1f via fmt would be fine but avoids fmt import
	n := int(f*10 + 0.5)
	hi := n / 10
	lo := n % 10
	b.WriteString(intToStr(hi))
	b.WriteString(".")
	b.WriteString(intToStr(lo))
	return b.String()
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func fmtInt(n int) string { return intToStr(n) }

// latencyTracker records samples of operation latency and computes p50/p99.
type latencyTracker struct {
	mu      sync.Mutex
	samples []time.Duration
}

func (t *latencyTracker) Record(d time.Duration) {
	t.mu.Lock()
	t.samples = append(t.samples, d)
	t.mu.Unlock()
}

func (t *latencyTracker) Report() (p50, p99, max time.Duration, n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.samples) == 0 {
		return 0, 0, 0, 0
	}
	cp := make([]time.Duration, len(t.samples))
	copy(cp, t.samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	p50 = cp[len(cp)*50/100]
	p99 = cp[len(cp)*99/100]
	max = cp[len(cp)-1]
	n = len(cp)
	return
}

// simulatePlayer runs a realistic player behavior loop until stop is closed.
// Returns without t.Fatal'ing so scenarios can control their own failure
// semantics.
func simulatePlayer(c *LoadClient, seed int64, stop <-chan struct{}) {
	r := rand.New(rand.NewSource(seed))
	centerX := int64(r.Intn(40) - 20)
	centerY := int64(r.Intn(40) - 20)
	reqID := uint64(1)

	// Initial view update — this is what the real frontend sends continuously.
	_ = c.ViewUpdate(centerX, centerY, 32*ChunkSize, 32*ChunkSize)

	tickView := time.NewTicker(250 * time.Millisecond)
	defer tickView.Stop()
	tickReveal := time.NewTicker(time.Duration(800+r.Intn(400)) * time.Millisecond)
	defer tickReveal.Stop()
	tickMiniOpen := time.NewTicker(time.Duration(15+r.Intn(30)) * time.Second)
	defer tickMiniOpen.Stop()

	// Drain incoming messages in the background so the reader channel doesn't
	// clog and cause back-pressure that boots us off.
	drainStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-c.recvChan:
			case <-drainStop:
				return
			}
		}
	}()
	defer close(drainStop)

	for {
		select {
		case <-stop:
			return
		case <-c.done:
			return
		case <-tickView.C:
			// Slight drift to simulate panning.
			centerX += int64(r.Intn(3) - 1)
			centerY += int64(r.Intn(3) - 1)
			_ = c.ViewUpdate(centerX, centerY, 32*ChunkSize, 32*ChunkSize)
		case <-tickReveal.C:
			// Pick a random cell near center.
			cx := centerX + int64(r.Intn(3)-1)
			cy := centerY + int64(r.Intn(3)-1)
			cell := uint32(r.Intn(ChunkSize * ChunkSize))
			_ = c.Reveal(cx, cy, cell, reqID)
			reqID++
		case <-tickMiniOpen.C:
			// Occasionally open a full-screen minimap covering ~20x20 chunks.
			tiles := make([]*pb.TileRef, 0, 400)
			for dy := int32(-10); dy <= 10; dy++ {
				for dx := int32(-10); dx <= 10; dx++ {
					tiles = append(tiles, &pb.TileRef{X: int32(centerX) + dx, Y: int32(centerY) + dy})
				}
			}
			_ = c.MinimapSubscribe(tiles, 32)
			// Close after a few seconds.
			go func(ts []*pb.TileRef) {
				time.Sleep(time.Duration(3+r.Intn(5)) * time.Second)
				_ = c.MinimapUnsubscribe(ts)
			}(tiles)
		}
	}
}
