//go:build integration

package main

import (
	"testing"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
	"google.golang.org/protobuf/proto"
)

func TestLargestViewRetainsVisibleSubscriptions(t *testing.T) {
	_, url, cleanup := startLoadTestServer(t)
	defer cleanup()
	client, err := NewLoadClient(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Join("largeview"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := client.ViewUpdate(-3, 4, 40*ChunkSize); err != nil {
			t.Fatal(err)
		}
		if err := client.send(&pb.Msg{Payload: &pb.Msg_SeedRequest{SeedRequest: &pb.SeedRequest{
			ChunkIds: []*pb.ChunkID{{X: 100, Y: 100}},
		}}}); err != nil {
			t.Fatal(err)
		}
		chunks := 0
		deadline := time.After(5 * time.Second)
	waiting:
		for {
			select {
			case msg := <-client.recv:
				if region := msg.GetChunkRegionSync(); region != nil {
					data := &pb.ChunkRegion{}
					if err := proto.Unmarshal(region.Chunks, data); err != nil {
						t.Fatal(err)
					}
					chunks += len(data.Chunks)
				}
				if msg.GetSeedResponse() != nil {
					break waiting
				}
			case <-deadline:
				t.Fatal("timed out waiting for view update processing")
			}
		}
		if attempt == 0 && chunks != 56*56 {
			t.Fatalf("initial view sent %d chunks, want %d", chunks, 56*56)
		}
		if attempt == 1 && chunks != 0 {
			t.Fatalf("unchanged view resent %d chunks evicted from its own viewport", chunks)
		}
	}
}
