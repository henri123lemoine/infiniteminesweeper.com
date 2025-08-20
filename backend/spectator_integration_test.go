package main

import (
    "bytes"
    "compress/gzip"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/gorilla/websocket"
    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
    "google.golang.org/protobuf/proto"
)

// startTestServerWithFSM spins up HTTP with unified /ws route using FSM.
func startTestServerWithFSM(t *testing.T) (*Server, string, func()) {
    t.Helper()
    s := NewServer()
    s.proximityRadius = -1
    mux := http.NewServeMux()
    mux.HandleFunc("/ws", s.handleWebSocket)

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
    cleanup := func() { close(done); ts.Close() }
    return s, wsURL, cleanup
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
    _, wsURL, cleanup := startTestServerWithFSM(t)
    defer cleanup()

    // Connect to unified /ws endpoint - starts in SPECTATOR state automatically
    c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws: %v", err) }
    defer c.Close()

    // Send a view update to trigger region sync adds - spectators can do this
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

// TestFSMStateTransitions tests the new FSM: SPECTATOR -> PLAYER transitions
func TestFSMStateTransitions(t *testing.T) {
    _, wsURL, cleanup := startTestServerWithFSM(t)
    defer cleanup()

    // 1) Connect and start in SPECTATOR state
    c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws1: %v", err) }
    defer c1.Close()

    // Send ViewUpdate - should work in spectator mode
    vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
        ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, WidthCells: 64, HeightCells: 64,
    }}}
    if err := writePB(c1, vu); err != nil { t.Fatalf("write view as spectator: %v", err) }

    // 2) Transition to PLAYER state via Join message
    join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "testplayer", FlagID: 0}}}
    if err := writePB(c1, join); err != nil { t.Fatalf("write join: %v", err) }

    // Expect JoinAck response
    deadline := time.Now().Add(2 * time.Second)
    var joinSucceeded bool
    for time.Now().Before(deadline) {
        m, err := readPB(c1, 2*time.Second)
        if err != nil { t.Fatalf("read: %v", err) }
        if ack := m.GetJoinAck(); ack != nil {
            if !ack.Ok {
                t.Fatalf("join failed: %s", ack.Error)
            }
            joinSucceeded = true
            break
        }
    }
    if !joinSucceeded {
        t.Fatalf("did not receive successful JoinAck")
    }

    // 3) Test that Reveal messages work in PLAYER state
    reveal := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
        ChunkId: &pb.ChunkID{X: 0, Y: 0}, Cell: 0, RequestId: 12345,
    }}}
    if err := writePB(c1, reveal); err != nil { t.Fatalf("write reveal as player: %v", err) }

    // Should receive RevealAck (server processes the reveal)
    deadline = time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        m, err := readPB(c1, 2*time.Second)
        if err != nil { t.Fatalf("read reveal response: %v", err) }
        if ack := m.GetRevealAck(); ack != nil {
            return // success - reveal was processed
        }
    }
    t.Fatalf("did not receive RevealAck for reveal command")
}

// TestNameValidation tests that duplicate names are properly rejected
func TestNameValidation(t *testing.T) {
    _, wsURL, cleanup := startTestServerWithFSM(t)
    defer cleanup()

    // 1) First player claims a name
    c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws1: %v", err) }
    defer c1.Close()

    join1 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "uniquename", FlagID: 0}}}
    if err := writePB(c1, join1); err != nil { t.Fatalf("write join1: %v", err) }

    // Wait for successful join
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        m, err := readPB(c1, 2*time.Second)
        if err != nil { t.Fatalf("read join1 response: %v", err) }
        if ack := m.GetJoinAck(); ack != nil && ack.Ok {
            break
        }
    }

    // 2) Second player tries to use the same name
    c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial ws2: %v", err) }
    defer c2.Close()

    join2 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "uniquename", FlagID: 1}}}
    if err := writePB(c2, join2); err != nil { t.Fatalf("write join2: %v", err) }

    // Should receive failed JoinAck
    deadline = time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        m, err := readPB(c2, 2*time.Second)
        if err != nil { t.Fatalf("read join2 response: %v", err) }
        if ack := m.GetJoinAck(); ack != nil {
            if ack.Ok {
                t.Fatalf("expected join to fail for duplicate name, but it succeeded")
            }
            if !strings.Contains(ack.Error, "taken") {
                t.Fatalf("expected 'taken' error, got: %s", ack.Error)
            }
            return // success - duplicate name was properly rejected
        }
    }
    t.Fatalf("did not receive JoinAck for duplicate name attempt")
}

