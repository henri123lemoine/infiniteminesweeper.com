// stress spawns N WebSocket clients that each play realistic minesweeper:
// pan around, reveal cells, occasionally open the minimap. Use it alongside
// `make go-run` in another terminal to exercise the full network path
// (gzip + protobuf + broadcast fan-out) while you play in the browser.
//
//	go run ./tools/stress -n 20 -url ws://localhost:8080/ws
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

const chunkSize = 64

func main() {
	n := flag.Int("n", 10, "number of simulated clients")
	url := flag.String("url", "ws://localhost:8080/ws", "websocket URL")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("stress: spawning %d clients against %s", *n, *url)
	var wg sync.WaitGroup
	wg.Add(*n)
	for i := 0; i < *n; i++ {
		go func(seed int64) {
			defer wg.Done()
			for ctx.Err() == nil {
				if err := runClient(ctx.Done(), *url, fmt.Sprintf("stress-%d", seed), seed); err != nil {
					log.Printf("client %d: %v (reconnecting in 1s)", seed, err)
					time.Sleep(time.Second)
				}
			}
		}(int64(i))
	}
	<-ctx.Done()
	log.Printf("stress: stopping")
	wg.Wait()
}

func runClient(stop <-chan struct{}, url, name string, seed int64) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	sendCh := make(chan *pb.Msg, 64)
	recvCh := make(chan *pb.Msg, 64)
	done := make(chan struct{})
	defer close(done)

	go readLoop(conn, recvCh, done)
	go writeLoop(conn, sendCh, done)

	sendCh <- &pb.Msg{Payload: &pb.Msg_Join{Join: &pb.Join{Name: name}}}
	select {
	case msg := <-recvCh:
		if ack := msg.GetJoinAck(); ack == nil || !ack.Ok {
			return fmt.Errorf("join failed")
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("join timeout")
	case <-stop:
		return nil
	}

	r := rand.New(rand.NewSource(seed))
	cx, cy := int64(r.Intn(40)-20), int64(r.Intn(40)-20)
	reqID := uint64(1)
	sendCh <- viewUpdate(cx, cy)

	tickView := time.NewTicker(time.Duration(200+(seed*13)%100) * time.Millisecond)
	tickReveal := time.NewTicker(time.Duration(800+(seed*29)%400) * time.Millisecond)
	tickMini := time.NewTicker(time.Duration(15+seed%15) * time.Second)
	defer tickView.Stop()
	defer tickReveal.Stop()
	defer tickMini.Stop()

	// Drain incoming so writePump backpressure doesn't disconnect us.
	go func() {
		for range recvCh {
		}
	}()

	for {
		select {
		case <-stop:
			return nil
		case <-done:
			return fmt.Errorf("conn closed")
		case <-tickView.C:
			cx += int64(r.Intn(3) - 1)
			cy += int64(r.Intn(3) - 1)
			sendCh <- viewUpdate(cx, cy)
		case <-tickReveal.C:
			sendCh <- &pb.Msg{Payload: &pb.Msg_Reveal{Reveal: &pb.Reveal{
				ChunkId: &pb.ChunkID{X: cx, Y: cy}, Cell: uint32(r.Intn(4096)), RequestId: reqID,
			}}}
			reqID++
		case <-tickMini.C:
			tiles := make([]*pb.TileRef, 0, 441)
			for dy := int32(-10); dy <= 10; dy++ {
				for dx := int32(-10); dx <= 10; dx++ {
					tiles = append(tiles, &pb.TileRef{X: int32(cx) + dx, Y: int32(cy) + dy})
				}
			}
			sendCh <- &pb.Msg{Payload: &pb.Msg_MinimapSubscribe{MinimapSubscribe: &pb.SubscribeTiles{
				Tiles: tiles, Resolution: 32,
			}}}
			go func(ts []*pb.TileRef) {
				time.Sleep(5 * time.Second)
				select {
				case sendCh <- &pb.Msg{Payload: &pb.Msg_MinimapUnsubscribe{MinimapUnsubscribe: &pb.UnsubscribeTiles{Tiles: ts}}}:
				case <-done:
				}
			}(tiles)
		}
	}
}

func viewUpdate(cx, cy int64) *pb.Msg {
	return &pb.Msg{Payload: &pb.Msg_ViewUpdate{ViewUpdate: &pb.ViewUpdate{
		ChunkId: &pb.ChunkID{X: cx, Y: cy}, Cell: 2048, WidthCells: 32 * chunkSize, HeightCells: 32 * chunkSize,
	}}}
}

func readLoop(conn *websocket.Conn, out chan<- *pb.Msg, done chan struct{}) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-done:
			default:
				close(done)
			}
			return
		}
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(gz)
		gz.Close()
		var msg pb.Msg
		if proto.Unmarshal(raw, &msg) != nil {
			continue
		}
		select {
		case out <- &msg:
		case <-done:
			return
		default:
		}
	}
}

func writeLoop(conn *websocket.Conn, in <-chan *pb.Msg, done chan struct{}) {
	for {
		select {
		case <-done:
			return
		case msg := <-in:
			data, err := proto.Marshal(msg)
			if err != nil {
				continue
			}
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			gz.Write(data)
			gz.Close()
			if conn.WriteMessage(websocket.BinaryMessage, buf.Bytes()) != nil {
				return
			}
		}
	}
}
