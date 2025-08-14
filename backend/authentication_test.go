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

    hello1 := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{
        SessionToken: "", Name: "Alice", FlagID: 5,
    }}}
    if err := c1.WriteMessage(websocket.BinaryMessage, mustProto(hello1)); err != nil {
        t.Fatalf("write hello1: %v", err)
    }
    _, w1bytes, err := c1.ReadMessage()
    if err != nil { t.Fatalf("read welcome1: %v", err) }
    w1 := decodePBMsg(t, w1bytes).GetWelcome()
    if w1 == nil { t.Fatalf("expected Welcome message") }
    if w1.SessionToken == "" { t.Fatalf("server did not issue a session token") }
    if got, want := w1.Name, "Alice"; got != want { t.Fatalf("name=%q want %q", got, want) }
    if w1.Score != 0 { t.Fatalf("score=%d want 0", w1.Score) }
    token := w1.SessionToken

    // Second connection: reuse token and update profile
    c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial2: %v", err) }
    defer c2.Close()

    hello2 := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{
        SessionToken: token, Name: "Bob", FlagID: 7,
    }}}
    if err := c2.WriteMessage(websocket.BinaryMessage, mustProto(hello2)); err != nil {
        t.Fatalf("write hello2: %v", err)
    }
    _, w2bytes, err := c2.ReadMessage()
    if err != nil { t.Fatalf("read welcome2: %v", err) }
    w2 := decodePBMsg(t, w2bytes).GetWelcome()
    if w2 == nil { t.Fatalf("expected Welcome2") }
    if w2.SessionToken != token { t.Fatalf("token changed on reuse") }
    if got, want := w2.Name, "Bob"; got != want { t.Fatalf("name=%q want %q", got, want) }
    if w2.FlagID != 7 { t.Fatalf("flag=%d want 7", w2.FlagID) }

    // Internal state checks
    srv.stateMu.RLock()
    defer srv.stateMu.RUnlock()
    if len(srv.sessionTokens) == 0 { t.Fatalf("server did not persist session token") }
}

func TestWelcomeNoInternalIDLeak(t *testing.T) {
    _, wsURL, cleanup := startTestServer(t)
    defer cleanup()

    c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil { t.Fatalf("dial: %v", err) }
    defer c.Close()

    hello := &pb.Msg{Payload: &pb.Msg_Hello{Hello: &pb.Hello{Name: "X"}}}
    if err := c.WriteMessage(websocket.BinaryMessage, mustProto(hello)); err != nil {
        t.Fatalf("write: %v", err)
    }
    _, data, err := c.ReadMessage()
    if err != nil { t.Fatalf("read: %v", err) }
    msg := decodePBMsg(t, data)
    w := msg.GetWelcome()
    if w == nil { t.Fatalf("want Welcome") }
    // Intentionally only token + profile; no internal player ID field exists
    if w.SessionToken == "" { t.Fatalf("empty token") }
}

