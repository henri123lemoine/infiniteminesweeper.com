package main

import (
    "bytes"
    "compress/gzip"
    "io"
    "testing"

    "github.com/gorilla/websocket"
    pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
    "google.golang.org/protobuf/proto"
)

// helper to decode gzipped pb.Msg
func decodePBMsg(t *testing.T, data []byte) *pb.Msg {
    t.Helper()
    gz, err := gzip.NewReader(bytes.NewReader(data))
    if err != nil {
        t.Fatalf("gzip: %v", err)
    }
    raw, err := io.ReadAll(gz)
    gz.Close()
    if err != nil {
        t.Fatalf("read: %v", err)
    }
    var m pb.Msg
    if err := proto.Unmarshal(raw, &m); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    return &m
}

func TestAuthentication_NewAndReuseSessionToken(t *testing.T) {
    srv, wsURL, cleanup := startTestServer(t)
    defer cleanup()

    // First connection: no token -> new identity
    c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial1: %v", err) }
    defer c1.Close()

    join1 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{
        SessionToken: "", Name: "Alice", FlagID: 5,
    }}}
    if err := c1.WriteMessage(websocket.BinaryMessage, mustProto(join1)); err != nil {
        t.Fatalf("write join1: %v", err)
    }
    _, j1bytes, err := c1.ReadMessage()
    if err != nil { t.Fatalf("read joinAck1: %v", err) }
    j1 := decodePBMsg(t, j1bytes).GetJoinAck()
    if j1 == nil { t.Fatalf("expected JoinAck message") }
    if j1.SessionToken == "" { t.Fatalf("server did not issue a session token") }
    if got, want := j1.Name, "Alice"; got != want { t.Fatalf("name=%q want %q", got, want) }
    if j1.Score != 0 { t.Fatalf("score=%d want 0", j1.Score) }
    token := j1.SessionToken

    // Second connection: reuse token and update profile
    c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial2: %v", err) }
    defer c2.Close()

    join2 := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{
        SessionToken: token, Name: "Bob", FlagID: 7,
    }}}
    if err := c2.WriteMessage(websocket.BinaryMessage, mustProto(join2)); err != nil {
        t.Fatalf("write join2: %v", err)
    }
    _, j2bytes, err := c2.ReadMessage()
    if err != nil { t.Fatalf("read joinAck2: %v", err) }
    j2 := decodePBMsg(t, j2bytes).GetJoinAck()
    if j2 == nil { t.Fatalf("expected JoinAck2") }
    if j2.SessionToken != token { t.Fatalf("token changed on reuse") }
    if got, want := j2.Name, "Bob"; got != want { t.Fatalf("name=%q want %q", got, want) }
    if j2.FlagID != 7 { t.Fatalf("flag=%d want 7", j2.FlagID) }

    // Internal state checks
    srv.stateMu.RLock()
    defer srv.stateMu.RUnlock()
    if len(srv.sessionTokens) == 0 { t.Fatalf("server did not persist session token") }
}

func TestJoinAckNoInternalIDLeak(t *testing.T) {
    _, wsURL, cleanup := startTestServer(t)
    defer cleanup()

    c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial: %v", err) }
    defer c.Close()

    join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: "X"}}}
    if err := c.WriteMessage(websocket.BinaryMessage, mustProto(join)); err != nil {
        t.Fatalf("write: %v", err)
    }
    _, data, err := c.ReadMessage()
    if err != nil { t.Fatalf("read: %v", err) }
    msg := decodePBMsg(t, data)
    j := msg.GetJoinAck()
    if j == nil { t.Fatalf("want JoinAck") }
    // Intentionally only token + profile; no internal player ID field exists
    if j.SessionToken == "" { t.Fatalf("empty token") }
}

