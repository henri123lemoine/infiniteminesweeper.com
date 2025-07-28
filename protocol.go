package main

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
	pb "infinite-minesweeper/pb"
)

var zstdEncoder, _ = zstd.NewWriter(nil)
var zstdDecoder, _ = zstd.NewReader(nil)
var zstdMu sync.Mutex

func marshalEnvelope(e *pb.Envelope) ([]byte, error) {
	b, err := proto.Marshal(e)
	if err != nil {
		return nil, err
	}
	zstdMu.Lock()
	defer zstdMu.Unlock()
	return zstdEncoder.EncodeAll(b, make([]byte, 0, len(b))), nil
}

func readEnvelope(conn *websocket.Conn) (*pb.Envelope, error) {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	zstdMu.Lock()
	b, err := zstdDecoder.DecodeAll(data, nil)
	zstdMu.Unlock()
	if err != nil {
		return nil, err
	}
	var env pb.Envelope
	if err := proto.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

func writeEnvelope(conn *websocket.Conn, env *pb.Envelope) error {
	b, err := marshalEnvelope(env)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, b)
}
