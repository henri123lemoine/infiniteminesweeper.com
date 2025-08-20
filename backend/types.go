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

// TokenBucket for rate limiting seed requests (200/min)
type TokenBucket struct {
	tokens     int
	lastRefill time.Time
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
