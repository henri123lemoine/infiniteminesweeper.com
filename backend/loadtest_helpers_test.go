//go:build integration
// +build integration

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
	"google.golang.org/protobuf/proto"
)

// Minimal load-test infrastructure. The existing TestClient in
// multiplayer_integration_test.go uses t.Fatalf which is unsafe to call from
// goroutines and unwanted in load tests where transient failures are
// acceptable; LoadClient mirrors the same protocol but returns errors.

func startLoadTestServer(t testing.TB) (*Server, string, func()) {
	t.Helper()
	s := NewServer()
	s.proximityRadius = -1

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
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
	// Minimap broadcaster is an infinite-loop goroutine; intentionally leak
	// for the test binary's lifetime rather than complicate it with shutdown.
	go s.runMinimapBroadcaster()

	ts := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	return s, wsURL, func() { close(stop); ts.Close() }
}

type LoadClient struct {
	conn   *websocket.Conn
	sendMu sync.Mutex
	closed atomic.Bool
	recv   chan *pb.Msg
}

func NewLoadClient(wsURL string) (*LoadClient, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &LoadClient{conn: conn, recv: make(chan *pb.Msg, 1024)}
	go func() {
		defer c.closed.Store(true)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			gz, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				continue
			}
			raw, _ := io.ReadAll(gz)
			gz.Close()
			var msg pb.Msg
			if proto.Unmarshal(raw, &msg) == nil {
				select {
				case c.recv <- &msg:
				default:
				}
			}
		}
	}()
	return c, nil
}

func (c *LoadClient) send(msg *pb.Msg) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed.Load() {
		return io.ErrClosedPipe
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(data)
	gz.Close()
	return c.conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func (c *LoadClient) Join(name string) error {
	if err := c.send(&pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: name}}}); err != nil {
		return err
	}
	for {
		select {
		case msg := <-c.recv:
			if ack := msg.GetJoinAck(); ack != nil {
				if !ack.Ok {
					return fmt.Errorf("join rejected: %s", ack.Error)
				}
				return nil
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("join timeout")
		}
	}
}

func (c *LoadClient) ViewUpdate(cx, cy int64, cells uint32) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId: &pb.ChunkID{X: cx, Y: cy}, Cell: 2048, WidthCells: cells, HeightCells: cells,
	}}})
}

func (c *LoadClient) Reveal(cx, cy int64, cell uint32, reqID uint64) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
		ChunkId: &pb.ChunkID{X: cx, Y: cy}, Cell: cell, RequestId: reqID,
	}}})
}

func (c *LoadClient) MinimapSubscribe(tiles []*pb.TileRef, res uint32) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_MinimapSubscribe{MinimapSubscribe: &pb.SubscribeTiles{Tiles: tiles, Resolution: res}}})
}

func (c *LoadClient) MinimapUnsubscribe(tiles []*pb.TileRef) error {
	return c.send(&pb.Msg{Payload: &pb.Msg_MinimapUnsubscribe{MinimapUnsubscribe: &pb.UnsubscribeTiles{Tiles: tiles}}})
}

func (c *LoadClient) Drain() {
	for {
		select {
		case <-c.recv:
		default:
			return
		}
	}
}

func (c *LoadClient) Close() {
	if !c.closed.Swap(true) {
		c.conn.Close()
	}
}

// snapshot logs current memory/goroutine/state-map sizes via t.Logf. Inlined
// here to avoid a separate type; callers print and forget.
func snapshot(t testing.TB, s *Server, label string) (heapMB int64, goroutines int) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s.stateMu.RLock()
	chunks, miniSubs, miniTiles := len(s.chunks), 0, len(s.minimapTiles)
	for _, set := range s.minimapSubs {
		miniSubs += len(set)
	}
	s.stateMu.RUnlock()

	s.playersMu.RLock()
	players := len(s.players)
	s.playersMu.RUnlock()

	heapMB = int64(ms.HeapAlloc) / 1024 / 1024
	goroutines = runtime.NumGoroutine()
	t.Logf("%s: %dMB heap / %dMB sys | %d goroutines, %d players, %d chunks, %d minisubs, %d minitiles",
		label, heapMB, ms.Sys/1024/1024, goroutines, players, chunks, miniSubs, miniTiles)
	return
}
