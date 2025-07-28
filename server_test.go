package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestGenerateChunkSeedDeterminism(t *testing.T) {
	s := NewServer()
	s.secret = []byte("test-secret")
	id := ChunkID{X: 1, Y: -1}

	seed1 := s.generateChunkSeed(id)
	seed2 := s.generateChunkSeed(id)
	if seed1 != seed2 {
		t.Fatalf("seed mismatch: %d vs %d", seed1, seed2)
	}

	h := hmac.New(sha256.New, s.secret)
	binary.Write(h, binary.LittleEndian, id.X)
	binary.Write(h, binary.LittleEndian, id.Y)
	expected := binary.LittleEndian.Uint64(h.Sum(nil)[:8])
	if seed1 != expected {
		t.Fatalf("seed %d != expected %d", seed1, expected)
	}
}

func TestIsValidCoordinate(t *testing.T) {
	s := NewServer()
	for y := -1; y <= ChunkSize; y++ {
		for x := -1; x <= ChunkSize; x++ {
			want := x >= 0 && x < ChunkSize && y >= 0 && y < ChunkSize
			if s.isValidCoordinate(x, y) != want {
				t.Fatalf("coord (%d,%d) expected %v", x, y, want)
			}
		}
	}
}

func TestRevealIdempotency(t *testing.T) {
	s := NewServer()
	id := ChunkID{0, 0}
	ok1 := s.reveal(1, id, 3, 5)
	ok2 := s.reveal(1, id, 3, 5)
	if !ok1 {
		t.Fatal("first reveal should succeed")
	}
	if ok2 {
		t.Fatal("second reveal should fail")
	}
	if s.scores[1] != 1 {
		t.Fatalf("score = %d, want 1", s.scores[1])
	}
}

func TestBuildLeaderboard(t *testing.T) {
	s := NewServer()
	s.stateMu.Lock()
	s.scores[3] = 1200
	s.scores[1] = 20000
	s.scores[2] = 999
	s.playerNames[1] = "one"
	s.playerNames[2] = "two"
	s.playerNames[3] = "three"
	s.buildLeaderboardUnsafe()
	s.stateMu.Unlock()

	var lb struct {
		Type    string    `json:"type"`
		Version uint64    `json:"version"`
		Entries []lbEntry `json:"entries"`
	}
	if err := json.Unmarshal(s.lbJSON, &lb); err != nil {
		t.Fatalf("unmarshal leaderboard: %v", err)
	}
	if lb.Version != 1 {
		t.Fatalf("version = %d, want 1", lb.Version)
	}
	if len(lb.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(lb.Entries))
	}
	if lb.Entries[0].PlayerID != 1 || lb.Entries[0].Score != "20.0k" || lb.Entries[0].Name != "one" {
		t.Fatalf("first entry %+v", lb.Entries[0])
	}
	if lb.Entries[1].PlayerID != 3 || lb.Entries[1].Score != "1.2k" || lb.Entries[1].Name != "three" {
		t.Fatalf("second entry %+v", lb.Entries[1])
	}
	if lb.Entries[2].PlayerID != 2 || lb.Entries[2].Score != "999" || lb.Entries[2].Name != "two" {
		t.Fatalf("third entry %+v", lb.Entries[2])
	}
}

func TestTokenBucket(t *testing.T) {
	tb := TokenBucket{}
	for i := 0; i < 200; i++ {
		if !tb.Take() {
			t.Fatalf("take %d failed early", i)
		}
	}
	if tb.Take() {
		t.Fatal("expected exhausted bucket")
	}
	tb.tokens = 0
	tb.lastRefill = time.Now().Add(-30 * time.Second)
	if !tb.Take() {
		t.Fatal("expected refill after 30s")
	}
	if tb.tokens != 99 {
		t.Fatalf("tokens after take = %d, want 99", tb.tokens)
	}
}

func TestRevealContention(t *testing.T) {
	s := NewServer()
	id := ChunkID{0, 0}
	wg := sync.WaitGroup{}
	successes := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(pid int32) {
			defer wg.Done()
			successes <- s.reveal(pid, id, 10, 10)
		}(int32(i))
	}
	wg.Wait()
	close(successes)
	count := 0
	for ok := range successes {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("success count = %d, want 1", count)
	}
}

// startTestServer spins up an HTTP server with the WebSocket handler and
// leaderboard broadcaster. It returns the websocket URL and a cleanup function.
func startTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()

	s := NewServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Leaderboard broadcaster with faster cadence for tests
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.stateMu.Lock()
				if s.lbDirty || s.lbJSON == nil {
					s.buildLeaderboardUnsafe()
					s.lbDirty = false
				}
				lbJSON := s.lbJSON
				lbVer := s.lbVersion
				s.stateMu.Unlock()

				s.playersMu.RLock()
				for _, set := range s.players {
					for p := range set {
						if p.LastLBVersion == lbVer {
							continue
						}
						select {
						case p.Send <- lbJSON:
							p.LastLBVersion = lbVer
						default:
						}
					}
				}
				s.playersMu.RUnlock()
			case <-done:
				return
			}
		}
	}()

	ts := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	cleanup := func() {
		close(done)
		ts.Close()
	}

	return s, wsURL, cleanup
}

type leaderboardMsg struct {
	Type    string `json:"type"`
	Version uint64 `json:"version"`
	Entries []struct {
		PlayerID int32  `json:"playerId"`
		Name     string `json:"name"`
		Score    string `json:"score"`
	} `json:"entries"`
}

func readLeaderboard(c *websocket.Conn, timeout time.Duration) (*leaderboardMsg, error) {
	deadline := time.Now().Add(timeout)
	for {
		c.SetReadDeadline(deadline)
		_, data, err := c.ReadMessage()
		if err != nil {
			return nil, err
		}
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &base); err != nil {
			continue
		}
		if base.Type == "leaderboard" {
			var lb leaderboardMsg
			if err := json.Unmarshal(data, &lb); err != nil {
				return nil, err
			}
			return &lb, nil
		}
	}
}

// waitForRevealAck blocks until it sees a {"type":"revealAck"} message or the
// timeout elapses.  It ignores broadcasts, leaderboard pushes, etc.
func waitForRevealAck(c *websocket.Conn, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		c.SetReadDeadline(deadline)
		var m map[string]any
		if err := c.ReadJSON(&m); err != nil {
			return false, err
		}
		if m["type"] == "revealAck" {
			ok, _ := m["ok"].(bool)
			return ok, nil
		}
		// otherwise: broadcast / chunkSync / leaderboard → continue
	}
}

// TestFullStackIntegration spins up a real server, connects 10 WebSocket
// clients, exercises contention + unique reveals, and checks that:
//
//   - exactly one client won the contention race
//   - every client succeeded on its unique cell
//   - the leaderboard has a single top scorer
func TestFullStackIntegration(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	const clients = 10
	conns := make([]*websocket.Conn, clients)

	// Phase 0: connect & handshake
	for i := 0; i < clients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns[i] = conn

		hello := map[string]any{
			"type":     "hello",
			"playerId": 0,
			"name":     fmt.Sprintf("p%d", i),
		}
		if err := conn.WriteJSON(hello); err != nil {
			t.Fatalf("hello %d: %v", i, err)
		}

		var welcome struct {
			Type     string `json:"type"`
			PlayerID int32  `json:"playerId"`
		}
		if err := conn.ReadJSON(&welcome); err != nil {
			t.Fatalf("welcome %d: %v", i, err)
		}
		if welcome.Type != "welcome" {
			t.Fatalf("unexpected welcome %d: %+v", i, welcome)
		}

		// drain first leaderboard so it doesn't confuse later reads
		if _, err := readLeaderboard(conn, 2*time.Second); err != nil {
			t.Fatalf("initial lb %d: %v", i, err)
		}
	}

	// Phase 1: contention reveal – all hit (1,1) concurrently
	start := make(chan struct{})
	var wg sync.WaitGroup
	okFlags := make([]bool, clients)

	for idx, conn := range conns {
		wg.Add(1)
		go func(i int, c *websocket.Conn) {
			defer wg.Done()
			<-start
			reveal := RevealMessage{Type: "reveal", ChunkX: 0, ChunkY: 0, X: 1, Y: 1}
			if err := c.WriteJSON(reveal); err != nil {
				t.Errorf("write contention %d: %v", i, err)
				return
			}
			ok, err := waitForRevealAck(c, 2*time.Second)
			if err != nil {
				t.Errorf("ack contention %d: %v", i, err)
				return
			}
			okFlags[i] = ok
			t.Logf("contention[%d] ok=%v", i, ok)
		}(idx, conn)
	}
	close(start)
	wg.Wait()

	// exactly one winner expected
	contenders := 0
	for _, ok := range okFlags {
		if ok {
			contenders++
		}
	}
	if contenders != 1 {
		t.Fatalf("expected exactly 1 contention winner, got %d", contenders)
	}

	// Phase 2: each client reveals its own unique safe cell
	successes := 0
	for i, conn := range conns {
		reveal := RevealMessage{Type: "reveal", ChunkX: 0, ChunkY: 0, X: i, Y: i + 2} // (0,2) … (9,11)
		if err := conn.WriteJSON(reveal); err != nil {
			t.Fatalf("write unique %d: %v", i, err)
		}
		ok, err := waitForRevealAck(conn, 2*time.Second)
		if err != nil {
			t.Fatalf("ack unique %d: %v", i, err)
		}
		if ok {
			successes++
		}
		t.Logf("unique[%d] ok=%v", i, ok)
	}

	if successes != clients {
		t.Fatalf("expected %d successful unique reveals, got %d", clients, successes)
	}

	// Phase 3: verify leaderboard (force immediate rebuild)
	srv.stateMu.Lock()
	srv.buildLeaderboardUnsafe()
	srv.stateMu.Unlock()

	srv.stateMu.RLock()
	var max int32
	for _, sc := range srv.scores {
		if sc > max {
			max = sc
		}
	}
	maxTied := 0
	for _, sc := range srv.scores {
		if sc == max {
			maxTied++
		}
	}
	srv.stateMu.RUnlock()

	t.Logf("max score=%d shared_by=%d", max, maxTied)
	if maxTied != 1 {
		t.Fatalf("expected a single top scorer, got %d", maxTied)
	}

	// cleanup
	for _, c := range conns {
		c.Close()
	}
}
