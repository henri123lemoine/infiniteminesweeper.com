package main

import (
    "bytes"
    "compress/gzip"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"
    "time"

    "github.com/gorilla/websocket"
    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
    "google.golang.org/protobuf/proto"
)

// startTestServerWithSpectate spins up HTTP with both /ws and /spectate routes.
func startTestServerWithSpectate(t *testing.T) (*Server, string, string, func()) {
    t.Helper()
    s := NewServer()
    s.proximityRadius = -1
    mux := http.NewServeMux()
    mux.HandleFunc("/ws", s.handleWebSocket)
    mux.HandleFunc("/spectate", s.handleSpectateWebSocket)

    // Minimal leaderboard broadcaster so writePump has activity similar to prod
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
                lb := s.lbProto
                ver := s.lbVersion
                s.stateMu.Unlock()
                s.playersMu.RLock()
                for _, set := range s.players {
                    for p := range set {
                        if p.LastLBVersion == ver { continue }
                        select { case p.Send <- lb: p.LastLBVersion = ver; default: }
                    }
                }
                s.playersMu.RUnlock()
            case <-done:
                return
            }
        }
    }()

    ts := httptest.NewServer(mux)
    base := strings.TrimPrefix(ts.URL, "http")
    wsURL := "ws" + base + "/ws"
    spURL := "ws" + base + "/spectate"
    cleanup := func() { close(done); ts.Close() }
    return s, wsURL, spURL, cleanup
}

func writePB(conn *websocket.Conn, m *pb.Msg) error {
    b, err := proto.Marshal(m)
    if err != nil { return err }
    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    if _, err := gz.Write(b); err != nil { return err }
    if err := gz.Close(); err != nil { return err }
    return conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func readPB(conn *websocket.Conn, timeout time.Duration) (*pb.Msg, error) {
    deadline := time.Now().Add(timeout)
    for {
        conn.SetReadDeadline(deadline)
        _, data, err := conn.ReadMessage()
        if err != nil { return nil, err }
        gz, err := gzip.NewReader(bytes.NewReader(data))
        if err != nil { continue }
        raw, err := io.ReadAll(gz)
        gz.Close()
        if err != nil { continue }
        var m pb.Msg
        if err := proto.Unmarshal(raw, &m); err != nil { continue }
        return &m, nil
    }
}

func TestSpectatorReceivesChunkRegionSync(t *testing.T) {
    _, _, spectateURL, cleanup := startTestServerWithSpectate(t)
    defer cleanup()

    c, _, err := websocket.DefaultDialer.Dial(spectateURL, nil)
    if err != nil { t.Fatalf("dial spectate: %v", err) }
    defer c.Close()

    // Send a view update to trigger region sync adds
    vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
        ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, WidthCells: 128, HeightCells: 128,
    }}}
    if err := writePB(c, vu); err != nil { t.Fatalf("write view: %v", err) }

    // Expect a ChunkRegionSync (gzipped pb.Msg with that payload)
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        m, err := readPB(c, 2*time.Second)
        if err != nil { t.Fatalf("read: %v", err) }
        if cr := m.GetChunkRegionSync(); cr != nil {
            return // success
        }
    }
    t.Fatalf("did not receive ChunkRegionSync")
}

// These tests are optional guards: set EXPECT_SINGLE_CONN=1 or EXPECT_NO_SPECTATE_WITH_PLAYER=1
// to enforce stricter invariants and discover duplicate-connection issues.
func TestDuplicateConnectionsGuards(t *testing.T) {
    s, wsURL, spectateURL, cleanup := startTestServerWithSpectate(t)
    defer cleanup()

    // 1) Duplicate /ws connections for the same token
    c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws1: %v", err) }
    defer c1.Close()
    _ = writePB(c1, &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{Name: "dup"}}})
    m1, err := readPB(c1, 2*time.Second)
    if err != nil || m1.GetWelcome() == nil { t.Fatalf("welcome1: %v", err) }
    tok := m1.GetWelcome().SessionToken

    c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws2: %v", err) }
    defer c2.Close()
    _ = writePB(c2, &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{SessionToken: tok}}})
    if _, err := readPB(c2, 2*time.Second); err != nil { t.Fatalf("welcome2: %v", err) }

    // Count connections for that player
    s.playersMu.RLock()
    var pid uint32
    for id, set := range s.players {
        // Find the ID which has c1 or c2; heuristic: latest added often matches last welcome
        if len(set) > 0 { pid = id }
    }
    conns := len(s.players[pid])
    s.playersMu.RUnlock()

    if os.Getenv("EXPECT_SINGLE_CONN") == "1" && conns != 1 {
        t.Fatalf("expected single connection per token; got %d", conns)
    }

    // 2) Spectator + player concurrent
    cs, _, err := websocket.DefaultDialer.Dial(spectateURL, nil)
    if err != nil { t.Fatalf("dial spectate: %v", err) }
    defer cs.Close()
    _ = writePB(cs, &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
        ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, WidthCells: 64, HeightCells: 64,
    }}})
    time.Sleep(100 * time.Millisecond)

    // Now check total connections across all players (includes spectator space)
    s.playersMu.RLock()
    total := 0
    for _, set := range s.players { total += len(set) }
    s.playersMu.RUnlock()
    if os.Getenv("EXPECT_NO_SPECTATE_WITH_PLAYER") == "1" && total > 1 {
        t.Fatalf("expected no concurrent spectator+player connections; found %d", total)
    }
}

