package main

import (
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

// FlagEntry packs one placed flag into 8 bytes. A chunk's flags live in a
// cell-sorted slice (~9B/flag resident vs ~22B as a map entry) with
// binary-search lookups and cache-friendly in-order iteration.
type FlagEntry struct {
	Cell   uint16
	FlagID uint16
	Owner  uint32
}

type chunkFlags []FlagEntry

func (cf chunkFlags) search(cell uint32) (int, bool) {
	lo, hi := 0, len(cf)
	for lo < hi {
		mid := (lo + hi) / 2
		if uint32(cf[mid].Cell) < cell {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(cf) && uint32(cf[lo].Cell) == cell
}

func (cf chunkFlags) get(cell uint32) (Flag, bool) {
	if i, ok := cf.search(cell); ok {
		return Flag{FlagID: uint32(cf[i].FlagID), Owner: cf[i].Owner}, true
	}
	return Flag{}, false
}

func (cf chunkFlags) set(cell uint32, f Flag) chunkFlags {
	e := FlagEntry{Cell: uint16(cell), FlagID: uint16(f.FlagID), Owner: f.Owner}
	i, ok := cf.search(cell)
	if ok {
		cf[i] = e
		return cf
	}
	cf = append(cf, FlagEntry{})
	copy(cf[i+1:], cf[i:])
	cf[i] = e
	return cf
}

// ExplosionEntry records who revealed the mine at a cell. Same cell-sorted
// per-chunk layout as chunkFlags; revealed mines are rare so these stay tiny.
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
	ID          uint32
	Conn        *websocket.Conn
	Send        chan []byte
	Mailbox     chan func(*Player) // actor channel
	TokenBucket TokenBucket
	Name        string
	FlagID      uint32
	View        PlayerView

	// FSM state - determines what messages this client can send/receive
	State ClientState

	// Leaderboard version already sent (protected by mailbox now)
	LastLBVersion uint64

	// Scoring
	Score int32

	// outbound-drop counter (closes WS after 32 consecutive drops)
	dropMisses int

	done chan struct{} // closed when player is fully removed
}

// TokenBucket for rate limiting seed requests (200/min). Only touched from
// the owning connection's readPump goroutine, so no locking.
type TokenBucket struct {
	tokens     int
	lastRefill time.Time
}

const (
	seedRequestBucketCap = 200
	seedRequestRefillPer = time.Minute / seedRequestBucketCap
)

// take refills based on elapsed time, then consumes one token. Returns false
// when the bucket is empty (caller should drop the request).
func (tb *TokenBucket) take(now time.Time) bool {
	if tb.lastRefill.IsZero() {
		tb.lastRefill = now
	}
	if refill := int(now.Sub(tb.lastRefill) / seedRequestRefillPer); refill > 0 {
		tb.tokens += refill
		if tb.tokens > seedRequestBucketCap {
			tb.tokens = seedRequestBucketCap
		}
		tb.lastRefill = tb.lastRefill.Add(time.Duration(refill) * seedRequestRefillPer)
	}
	if tb.tokens <= 0 {
		return false
	}
	tb.tokens--
	return true
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
