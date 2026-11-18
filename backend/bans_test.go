package main

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

func TestParseBanList(t *testing.T) {
	b := newBanList()
	err := parseBanList(strings.NewReader(`
# comment
1234   # example
203.0.113.7
2001:db8:1234::/48
not-an-entry
`), b)
	if err != nil {
		t.Fatal(err)
	}
	if !b.bannedPlayer(1234) || b.bannedPlayer(1235) {
		t.Fatalf("player bans wrong: %v", b.players)
	}
	cases := map[string]bool{
		"203.0.113.7":        true,
		"::ffff:203.0.113.7": true,
		"203.0.113.8":        false,
		"2001:db8:1234:5::1": true,
		"2001:db8:1235::1":   false,
	}
	for s, want := range cases {
		if got := b.bannedIP(netip.MustParseAddr(s)); got != want {
			t.Errorf("bannedIP(%s)=%v want %v", s, got, want)
		}
	}
	var nilList *banList
	if nilList.bannedPlayer(1) || nilList.bannedIP(netip.MustParseAddr("1.2.3.4")) || nilList.stillBanned("p1") {
		t.Fatal("nil list must ban nothing")
	}
}

func TestClientIPTrustsFlyHeaderOnlyOnFly(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("Fly-Client-IP", "203.0.113.7")

	t.Setenv("FLY_APP_NAME", "")
	if got := clientIP(r).String(); got != "10.0.0.1" {
		t.Fatalf("off Fly got %s, want RemoteAddr", got)
	}
	t.Setenv("FLY_APP_NAME", "infiniteminesweeper")
	if got := clientIP(r).String(); got != "203.0.113.7" {
		t.Fatalf("on Fly got %s, want Fly-Client-IP", got)
	}
}

func writeBans(t *testing.T, srv *Server, body string) {
	t.Helper()
	if srv.dataDir == "data" {
		srv.dataDir = t.TempDir()
	}
	if err := os.WriteFile(filepath.Join(srv.dataDir, bansFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := srv.loadBans(); err != nil {
		t.Fatal(err)
	}
}

func autoBans(t *testing.T, srv *Server) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(srv.dataDir, autoBansFileName))
	return string(b)
}

// joinAs returns the first JoinAck or Banned the server sends.
func joinAs(t *testing.T, wsURL string, hdr http.Header, token, name string) *pb.Msg {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: token, Name: name}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(join)); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if m := decodePBMsg(t, data); m.GetJoinAck() != nil || m.GetBanned() != nil {
			return m
		}
	}
}

func onFlyFrom(t *testing.T, ip string) http.Header {
	t.Setenv("FLY_APP_NAME", "test")
	return http.Header{"Fly-Client-IP": {ip}}
}

func TestBannedPlayerRejoinGetsBanPageAndIPBan(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	hdr := onFlyFrom(t, "203.0.113.9")

	first := joinAs(t, wsURL, hdr, "", "Tester").GetJoinAck()
	if first == nil || !first.Ok {
		t.Fatalf("initial join failed: %v", first)
	}
	srv.stateMu.RLock()
	pid := srv.sessionTokens[first.SessionToken]
	srv.stateMu.RUnlock()

	writeBans(t, srv, strconv.FormatUint(uint64(pid), 10)+"\n")
	m := joinAs(t, wsURL, hdr, first.SessionToken, "Tester")
	if m.GetBanned() == nil || !strings.Contains(m.GetBanned().Html, "personal project") {
		t.Fatalf("banned rejoin got %v", m)
	}
	if !strings.Contains(autoBans(t, srv), "203.0.113.9/32") {
		t.Fatalf("rejoin IP not auto-banned: %q", autoBans(t, srv))
	}
}

func TestBannedIPGetsBannedMessageAndCookie(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	writeBans(t, srv, "203.0.113.7\n")

	c, resp, err := websocket.DefaultDialer.Dial(wsURL, onFlyFrom(t, "203.0.113.7"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if !strings.Contains(resp.Header.Get("Set-Cookie"), banCookieName+"=203.0.113.7/32") {
		t.Fatalf("no ban cookie on upgrade: %q", resp.Header.Get("Set-Cookie"))
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if decodePBMsg(t, data).GetBanned() == nil {
		t.Fatal("first message was not Banned")
	}
}

func TestBanGate(t *testing.T) {
	srv := NewServer()
	srv.dataDir = t.TempDir()
	writeBans(t, srv, "42\n203.0.113.7\n")
	t.Setenv("FLY_APP_NAME", "test")
	gate := srv.banGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("game"))
	}))
	get := func(ip string, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/how-to-play/", nil)
		r.Header.Set("Fly-Client-IP", ip)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: banCookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, r)
		return w
	}

	if w := get("198.51.100.1", ""); w.Code != http.StatusOK || w.Body.String() != "game" {
		t.Fatalf("clean visitor blocked: %d", w.Code)
	}
	w := get("203.0.113.7", "")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Bots and automated play") {
		t.Fatalf("banned IP not shown ban page: %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), banCookieName+"=203.0.113.7/32") {
		t.Fatalf("ban page did not set cookie: %q", w.Header().Get("Set-Cookie"))
	}

	// A banned browser on a new network bans that network too, IPv6 by /64.
	if w := get("2001:db8:aa:bb::5", "p42"); w.Code != http.StatusForbidden {
		t.Fatalf("cookie evasion not blocked: %d", w.Code)
	}
	if !strings.Contains(autoBans(t, srv), "2001:db8:aa:bb::/64 # evaded p42") {
		t.Fatalf("evading IP not auto-banned: %q", autoBans(t, srv))
	}
	if w := get("2001:db8:aa:bb::99", ""); w.Code != http.StatusForbidden {
		t.Fatalf("auto-banned /64 not blocked: %d", w.Code)
	}

	// Forged or stale cookies ban nothing.
	if w := get("198.51.100.2", "p43"); w.Code != http.StatusOK {
		t.Fatalf("cookie for unlisted player blocked: %d", w.Code)
	}
	writeBans(t, srv, "203.0.113.7\n")
	if w := get("198.51.100.3", "p42"); w.Code != http.StatusOK {
		t.Fatalf("unbanned player's cookie still blocks: %d", w.Code)
	}
}

func TestBannedPlayerHiddenFromLeaderboards(t *testing.T) {
	srv := NewServer()
	srv.scores[1], srv.playerNames[1] = 1000, "cheater"
	srv.scores[2], srv.playerNames[2] = 10, "honest"
	writeBans(t, srv, "1\n")

	srv.stateMu.Lock()
	defer srv.stateMu.Unlock()
	for _, e := range srv.collectLeaderboardLocked() {
		if e.Name == "cheater" {
			t.Fatal("banned player on HTTP leaderboard")
		}
	}
	if r := srv.getUserRankUnsafe(10); r != 1 {
		t.Fatalf("rank=%d want 1 with the banned score excluded", r)
	}
}

// playAs joins and sends n reveals in bursts small enough that the acks
// don't trip the server's back-pressure disconnect. It reports the session
// token and whether the server answered with Banned.
func playAs(t *testing.T, wsURL string, hdr http.Header, token string, n int) (string, bool) {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	join := &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{SessionToken: token, Name: "Clicker"}}}
	if err := c.WriteMessage(websocket.BinaryMessage, mustProto(join)); err != nil {
		t.Fatal(err)
	}
	joined := make(chan string, 1)
	banned := make(chan struct{})
	go func() {
		c.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			m := decodePBMsg(t, data)
			if a := m.GetJoinAck(); a != nil {
				joined <- a.SessionToken
			}
			if m.GetBanned() != nil {
				close(banned)
				return
			}
		}
	}()
	select {
	case token = <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("no JoinAck")
	}
	for i := range n {
		m := &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
			RequestId: uint64(i), ChunkId: &pb.ChunkID{}, Cell: uint32(i % 4096),
		}}}
		if err := c.WriteMessage(websocket.BinaryMessage, mustProto(m)); err != nil {
			t.Fatalf("reveal %d: %v", i, err)
		}
		if i%50 == 49 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	select {
	case <-banned:
		return token, true
	case <-time.After(500 * time.Millisecond):
		return token, false
	}
}

func TestSustainedRevealRateGetsBannedAcrossReconnects(t *testing.T) {
	srv, wsURL, cleanup := startTestServer(t)
	defer cleanup()
	srv.dataDir = t.TempDir()
	hdr := onFlyFrom(t, "203.0.113.20")

	token, banned := playAs(t, wsURL, hdr, "", botBucketCap/2)
	if banned {
		t.Fatalf("banned after only %d reveals", botBucketCap/2)
	}
	if _, banned = playAs(t, wsURL, hdr, token, botBucketCap/2+150); !banned {
		t.Fatal("no Banned message after draining the allowance over two connections")
	}
	srv.stateMu.RLock()
	pid := srv.sessionTokens[token]
	srv.stateMu.RUnlock()

	got := autoBans(t, srv)
	if !strings.Contains(got, strconv.FormatUint(uint64(pid), 10)+" # bot") || !strings.Contains(got, "203.0.113.20/32 # bot") {
		t.Fatalf("bot not auto-banned by ID and IP: %q", got)
	}
	if !srv.isPlayerBanned(pid) {
		t.Fatal("ban list not reloaded")
	}
}

func TestOnlyBeyondHumanUninterruptedPlayIsOverHumanRate(t *testing.T) {
	srv := &Server{}
	now := time.Now()
	// Faster than any human clicks, held for an hour: still not flagged.
	for i := range 15 * 3600 {
		if _, over := srv.overHumanRate(1, now.Add(time.Duration(i)*time.Second/15)); over {
			t.Fatalf("15 reveals/s was flagged after %d reveals", i)
		}
	}
	// The rate limit's ceiling, but with a pause every minute.
	for i := range 40 * 600 {
		at := now.Add(time.Duration(i) * time.Second / 40).Add(time.Duration(i/(40*50)) * botPauseReset)
		if _, over := srv.overHumanRate(2, at); over {
			t.Fatalf("40 reveals/s with a pause every 50s was flagged after %d reveals", i)
		}
	}
	for i := range 40 * 90 {
		at := now.Add(time.Duration(i) * time.Second / 40)
		if run, over := srv.overHumanRate(3, at); over {
			if at.Sub(now) < 55*time.Second {
				t.Fatalf("40 reveals/s flagged after only %v", at.Sub(now))
			}
			if !strings.Contains(run, "reveals in 60s") {
				t.Fatalf("run description = %q", run)
			}
			return
		}
	}
	t.Fatal("40 reveals/s for 90s without a pause was not flagged")
}
