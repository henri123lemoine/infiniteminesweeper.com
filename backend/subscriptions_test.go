package main

import (
    "testing"
    "time"

    "github.com/gorilla/websocket"
    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestViewUpdateSubscriptionsAndLRU(t *testing.T) {
    srv, wsURL, cleanup := startTestServer(t)
    defer cleanup()

    // Shrink capacity to make eviction easy to observe
    srv.stateMu.Lock()
    srv.maxPlayerSubs = 20
    srv.stateMu.Unlock()

    c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial: %v", err) }
    defer c.Close()

    // handshake
    hello := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{Name: "SubTester"}}}
    if err := c.WriteMessage(websocket.BinaryMessage, mustProto(hello)); err != nil {
        t.Fatalf("write hello: %v", err)
    }
    if _, _, err := c.ReadMessage(); err != nil { t.Fatalf("welcome: %v", err) }
    // drain one leaderboard if present (best-effort)
    _ = func() error { _, _, err := c.ReadMessage(); return err }()

    // send a view covering 4x4 chunks → expect <= 16 subs
    vu := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
        ChunkId: &pb.ChunkID{X: 0, Y: 0},
        Cell: 0, WidthCells: 128, HeightCells: 128,
    }}}
    if err := c.WriteMessage(websocket.BinaryMessage, mustProto(vu)); err != nil {
        t.Fatalf("view1: %v", err)
    }

    // allow readPump to process
    time.Sleep(100 * time.Millisecond)

    // fetch the single playerID
    srv.playersMu.RLock()
    var pid uint32
    for k := range srv.players { pid = k; break }
    srv.playersMu.RUnlock()
    if pid == 0 { t.Fatalf("no player registered") }

    srv.stateMu.RLock()
    subCount := len(srv.playerSubs[pid])
    srv.stateMu.RUnlock()
    if subCount == 0 || subCount > 16 {
        t.Fatalf("unexpected subs after view: %d (want 1..16)", subCount)
    }

    // Move view far away to trigger LRU evictions; send a larger rect 5x5 => 25 but capacity=20
    vu2 := &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
        ChunkId: &pb.ChunkID{X: 50, Y: -50},
        Cell: 0, WidthCells: 256, HeightCells: 256,
    }}}
    if err := c.WriteMessage(websocket.BinaryMessage, mustProto(vu2)); err != nil {
        t.Fatalf("view2: %v", err)
    }
    time.Sleep(100 * time.Millisecond)

    srv.stateMu.RLock()
    subCount2 := len(srv.playerSubs[pid])
    srv.stateMu.RUnlock()
    if subCount2 == 0 || subCount2 > srv.maxPlayerSubs {
        t.Fatalf("LRU not enforced: subs=%d cap=%d", subCount2, srv.maxPlayerSubs)
    }
}
