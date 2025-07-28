package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"

	"infinite-minesweeper/pb"
)

var zdec, _ = zstd.NewReader(nil)
var zenc, _ = zstd.NewWriter(nil)

func decodeEnv(data []byte) (*pb.Envelope, error) {
	d, err := zdec.DecodeAll(data, nil)
	if err != nil {
		return nil, err
	}
	var env pb.Envelope
	if err := proto.Unmarshal(d, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

func sendEnv(conn *websocket.Conn, env *pb.Envelope) error {
	b, err := proto.Marshal(env)
	if err != nil {
		return err
	}
	c := zenc.EncodeAll(b, nil)
	return conn.WriteMessage(websocket.BinaryMessage, c)
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

	var env pb.Envelope
	mustDecode(s.lbJSON, &env)
	lb := env.GetLeaderboard()
	if lb.Version != 1 {
		t.Fatalf("version = %d, want 1", lb.Version)
	}
	if len(lb.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(lb.Entries))
	}
	if lb.Entries[0].PlayerId != 1 || lb.Entries[0].Score != "20.0k" || lb.Entries[0].Name != "one" {
		t.Fatalf("first entry %+v", lb.Entries[0])
	}
	if lb.Entries[1].PlayerId != 3 || lb.Entries[1].Score != "1.2k" || lb.Entries[1].Name != "three" {
		t.Fatalf("second entry %+v", lb.Entries[1])
	}
	if lb.Entries[2].PlayerId != 2 || lb.Entries[2].Score != "999" || lb.Entries[2].Name != "two" {
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
	Version uint64
	Entries []struct {
		PlayerID int32
		Name     string
		Score    string
	}
}

func readLeaderboard(c *websocket.Conn, timeout time.Duration) (*leaderboardMsg, error) {
	deadline := time.Now().Add(timeout)
	for {
		c.SetReadDeadline(deadline)
		_, data, err := c.ReadMessage()
		if err != nil {
			return nil, err
		}
		env, err := decodeEnv(data)
		if err != nil {
			continue
		}
		lb := env.GetLeaderboard()
		if lb != nil {
			out := &leaderboardMsg{Version: lb.Version}
			for _, e := range lb.Entries {
				out.Entries = append(out.Entries, struct {
					PlayerID int32
					Name     string
					Score    string
				}{e.PlayerId, e.Name, e.Score})
			}
			return out, nil
		}
	}
}

func TestFullStackIntegration(t *testing.T) {
	t.Skip("integration test disabled")
	s, wsURL, cleanup := startTestServer(t)
	defer cleanup()

	const clients = 10
	conns := make([]*websocket.Conn, clients)

	for i := 0; i < clients; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns[i] = c
		hello := &pb.Envelope{Msg: &pb.Envelope_Hello{Hello: &pb.Hello{PlayerId: 0, Name: fmt.Sprintf("p%d", i)}}}
		if err := sendEnv(c, hello); err != nil {
			t.Fatalf("hello %d: %v", i, err)
		}
		// welcome
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("welcome %d: %v", i, err)
		}
		env, err := decodeEnv(data)
		if err != nil {
			t.Fatalf("decode welcome %d: %v", i, err)
		}
		if env.GetWelcome() == nil {
			t.Fatalf("welcome %d: %v", i, err)
		}
		if _, err := readLeaderboard(c, time.Second); err != nil {
			t.Fatalf("initial lb %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	wg := sync.WaitGroup{}
	winners := make(chan int, clients)
	for i, c := range conns {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			<-start
			msg := &pb.Envelope{Msg: &pb.Envelope_Reveal{Reveal: &pb.RevealRequest{ChunkX: 0, ChunkY: 0, X: 1, Y: 1}}}
			if err := sendEnv(conn, msg); err != nil {
				t.Errorf("write reveal %d: %v", idx, err)
				return
			}
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("read ack %d: %v", idx, err)
				return
			}
			env, err := decodeEnv(data)
			if err != nil {
				t.Errorf("decode ack %d: %v", idx, err)
				return
			}
			if ack := env.GetRevealAck(); ack != nil && ack.Ok {
				winners <- idx
			}
		}(i, c)
	}
	close(start)
	wg.Wait()
	close(winners)

	var winIdx int
	count := 0
	for w := range winners {
		winIdx = w
		count++
	}
	if count != 1 {
		t.Fatalf("expected 1 winner, got %d", count)
	}

	for i, c := range conns {
		msg := &pb.Envelope{Msg: &pb.Envelope_Reveal{Reveal: &pb.RevealRequest{ChunkX: 0, ChunkY: 0, X: int32(i), Y: int32(i + 2)}}}
		if err := sendEnv(c, msg); err != nil {
			t.Fatalf("write unique %d: %v", i, err)
		}
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read ack unique %d: %v", i, err)
		}
		env, err := decodeEnv(data)
		if err != nil {
			t.Fatalf("decode ack unique %d: %v", i, err)
		}
		if ack := env.GetRevealAck(); ack == nil || !ack.Ok {
			t.Fatalf("unique reveal %d failed", i)
		}
	}

	lb, err := readLeaderboard(conns[winIdx], 3*time.Second)
	if err != nil {
		t.Fatalf("read leaderboard: %v", err)
	}
	if len(lb.Entries) != clients {
		t.Fatalf("entries = %d, want %d", len(lb.Entries), clients)
	}
	if lb.Entries[0].Score != "2" {
		t.Fatalf("top score = %s, want 2", lb.Entries[0].Score)
	}
	for i, e := range lb.Entries[1:] {
		if e.Score != "1" {
			t.Fatalf("score idx %d = %s, want 1", i+1, e.Score)
		}
	}

	for _, c := range conns {
		c.Close()
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if len(s.scores) != clients {
		t.Fatalf("score map len = %d, want %d", len(s.scores), clients)
	}
}
