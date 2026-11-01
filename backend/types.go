package main

import (
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

// ClientState represents the finite state machine state for a client
type ClientState int32

const (
	ClientStateSpectator ClientState = 0 // Can only view, no game actions
	ClientStatePlayer    ClientState = 1 // Full game capabilities
)

type ChunkID struct {
	X, Y int64
}

type ChunkBits [64]uint64

type Flag struct {
	FlagID uint32
	// PlayerID of whoever placed it; 0 for flags that predate ownership tracking.
	Owner uint32
}

// FlagEntry is one placed flag in the snapshot encoding (8 bytes on disk).
// In memory a chunk's flags are a chunkFlags; see flags.go.
type FlagEntry struct {
	Cell   uint16
	FlagID uint16
	Owner  uint32
}

// ExplosionEntry records who revealed the mine at a cell. A chunk's entries
// are a cell-sorted slice; revealed mines are rare so these stay tiny.
type ExplosionEntry struct {
	Cell  uint16
	Owner uint32
}

type chunkExplosions []ExplosionEntry

func (ce chunkExplosions) search(cell uint32) (int, bool) {
	lo, hi := 0, len(ce)
	for lo < hi {
		mid := (lo + hi) / 2
		if uint32(ce[mid].Cell) < cell {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(ce) && uint32(ce[lo].Cell) == cell
}

func (ce chunkExplosions) get(cell uint32) (uint32, bool) {
	if i, ok := ce.search(cell); ok {
		return ce[i].Owner, true
	}
	return 0, false
}

func (ce chunkExplosions) set(cell uint32, owner uint32) chunkExplosions {
	e := ExplosionEntry{Cell: uint16(cell), Owner: owner}
	i, ok := ce.search(cell)
	if ok {
		ce[i] = e
		return ce
	}
	ce = append(ce, ExplosionEntry{})
	copy(ce[i+1:], ce[i:])
	ce[i] = e
	return ce
}

type PlayerView struct {
	Chunk ChunkID
	Cell  uint32
	// Last region dimensions in chunks used for subscription window
	RectWChunks int
	RectHChunks int
}

type Player struct {
	ID      uint32
	Conn    *websocket.Conn
	Send    chan []byte
	Mailbox chan func(*Player) // actor channel
	Name    string
	FlagID  uint32
	View    PlayerView

	// FSM state - determines what messages this client can send/receive
	State ClientState

	// Leaderboard version already sent (protected by mailbox now)
	LastLBVersion uint64

	// Scoring
	Score int32

	// outbound-drop counter (closes WS after 32 consecutive drops)
	dropMisses int

	// Readpump-owned: chunk subscription budget and the last view rect it
	// fully covered, so repeated identical ViewUpdates skip the state lock.
	chunkBucket   TokenBucket
	lastViewRect  [4]int64
	lastViewValid bool

	// Set when a newer login for the same player takes over this connection.
	displaced atomic.Bool

	done chan struct{} // closed when player is fully removed
}

// TokenBucket is not safe for concurrent use; the owner provides locking.
type TokenBucket struct {
	capacity    int
	refillEvery time.Duration
	tokens      int
	lastRefill  time.Time
}

type rateLimit struct {
	capacity    int
	refillEvery time.Duration
}

// Reveals, flags and chords each cost one action token. Chunk tokens are
// spent per newly subscribed chunk; the burst covers one full
// max-zoom-out view (56x56 chunks).
var (
	defaultActionLimit = rateLimit{capacity: 40, refillEvery: time.Second / 10}
	defaultChunkLimit  = rateLimit{capacity: 4096, refillEvery: time.Second / 400}
)

// Rate-limited actions are strikes; a streak of strikes with no quiet gap of
// resetAfter suspends the player. A human needs >10 clicks/s sustained well
// past the burst to get anywhere near this.
type suspensionPolicy struct {
	strikes    int
	resetAfter time.Duration
	duration   time.Duration
}

var defaultSuspension = suspensionPolicy{strikes: 100, resetAfter: 10 * time.Second, duration: time.Hour}

type actionBudget struct {
	bucket         TokenBucket
	strikes        int
	lastStrike     time.Time
	suspendedUntil time.Time
	releasePending bool
}

// freshAt is when this budget becomes indistinguishable from a new one, so it
// can be dropped without letting a reconnect skip a refill or suspension.
func (b *actionBudget) freshAt(resetAfter time.Duration) time.Time {
	t := b.suspendedUntil
	tb := b.bucket
	if full := tb.lastRefill.Add(time.Duration(tb.capacity-tb.tokens) * tb.refillEvery); full.After(t) {
		t = full
	}
	if b.strikes > 0 {
		if r := b.lastStrike.Add(resetAfter); r.After(t) {
			t = r
		}
	}
	return t
}

func newTokenBucket(l rateLimit) TokenBucket {
	return TokenBucket{capacity: l.capacity, refillEvery: l.refillEvery, tokens: l.capacity}
}

func (tb *TokenBucket) refill(now time.Time) {
	if tb.lastRefill.IsZero() {
		tb.lastRefill = now
		return
	}
	n := int(now.Sub(tb.lastRefill) / tb.refillEvery)
	if n <= 0 {
		return
	}
	tb.tokens = min(tb.tokens+n, tb.capacity)
	if tb.tokens == tb.capacity {
		tb.lastRefill = now
	} else {
		tb.lastRefill = tb.lastRefill.Add(time.Duration(n) * tb.refillEvery)
	}
}

// take consumes one token, or returns false when the bucket is empty.
func (tb *TokenBucket) take(now time.Time) bool {
	return tb.takeUpTo(now, 1) == 1
}

// takeUpTo consumes as many of n tokens as are available and returns how
// many it got.
func (tb *TokenBucket) takeUpTo(now time.Time, n int) int {
	tb.refill(now)
	got := min(n, tb.tokens)
	if got < 0 {
		got = 0
	}
	tb.tokens -= got
	return got
}

// Convert internal ClientState to protobuf ClientState
func (cs ClientState) ToPB() pb.ClientState {
	switch cs {
	case ClientStateSpectator:
		return pb.ClientState_SPECTATOR
	case ClientStatePlayer:
		return pb.ClientState_PLAYER
	default:
		return pb.ClientState_SPECTATOR // default to spectator for safety
	}
}

// Convert protobuf ClientState to internal ClientState
func ClientStateFromPB(pbState pb.ClientState) ClientState {
	switch pbState {
	case pb.ClientState_SPECTATOR:
		return ClientStateSpectator
	case pb.ClientState_PLAYER:
		return ClientStatePlayer
	default:
		return ClientStateSpectator // default to spectator for safety
	}
}
