package main

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func encodeMsg(m *pb.Msg) []byte {
	return mustProto(m)
}

func decodeMsg(data []byte) (*pb.Msg, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(gz)
	gz.Close()
	if err != nil {
		return nil, err
	}

	var m pb.Msg
	if err := proto.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

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

	msg, err := decodeMsg(s.lbProto)
	if err != nil {
		t.Fatalf("decode leaderboard: %v", err)
	}
	lb := msg.GetLeaderboard()
	if lb == nil {
		t.Fatalf("no leaderboard")
	}
	if lb.Version != 1 {
		t.Fatalf("version = %d, want 1", lb.Version)
	}
	if len(lb.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(lb.Entries))
	}
	if lb.Entries[0].Name != "one" || lb.Entries[0].Score != 20000 {
		t.Fatalf("first entry %+v", lb.Entries[0])
	}
	if lb.Entries[1].Name != "three" || lb.Entries[1].Score != 1200 {
		t.Fatalf("second entry %+v", lb.Entries[1])
	}
	if lb.Entries[2].Name != "two" || lb.Entries[2].Score != 999 {
		t.Fatalf("third entry %+v", lb.Entries[2])
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

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"abc", true},
		{"A_B-C123", true},
		{"ab", true},
		{"thisnameiswaaaaaaaaaaaaaaaaaaaaaaaaaaaaaytoolongforvalidation", false},
		{"bad!name", false},
		{"space name", true},
		{"", false},
	}
	for _, tt := range tests {
		if isValidUsername(tt.name) != tt.valid {
			t.Errorf("isValidUsername(%q)=%v, want %v", tt.name, isValidUsername(tt.name), tt.valid)
		}
	}
}

// startTestServer spins up an HTTP server with the WebSocket handler and
// leaderboard broadcaster. It returns the websocket URL and a cleanup function.
func startTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()

	s := NewServer()
	// Disable proximity rule for tests to match legacy behavior
	s.proximityRadius = -1
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
				if s.lbDirty || s.lbProto == nil {
					s.buildLeaderboardUnsafe()
					s.lbDirty = false
				}
				lbJSON := s.lbProto
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
		Name  string `json:"name"`
		Score int32  `json:"score"`
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
		msg, err := decodeMsg(data)
		if err != nil {
			continue
		}
		if lb := msg.GetLeaderboard(); lb != nil {
			out := &leaderboardMsg{Type: "leaderboard", Version: lb.Version}
			for _, e := range lb.Entries {
				out.Entries = append(out.Entries, struct {
					Name  string `json:"name"`
					Score int32  `json:"score"`
				}{e.Name, e.Score})
			}
			return out, nil
		}
	}
}

// waitForRevealAck blocks until it sees a {"type":"revealAck"} message or the
// timeout elapses.  It ignores broadcasts, leaderboard pushes, etc.
func waitForRevealAck(c *websocket.Conn, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		c.SetReadDeadline(deadline)
		_, data, err := c.ReadMessage()
		if err != nil {
			return false, err
		}
		m, err := decodeMsg(data)
		if err != nil {
			continue
		}
		if ack := m.GetRevealAck(); ack != nil {
			return ack.Ok, nil
		}
		// otherwise: broadcast / chunkSync / leaderboard → continue
	}
}
