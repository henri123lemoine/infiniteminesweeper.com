package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	s := NewServer()
	s.proximityRadius = -1
	// Pre-populate a tiny bit of state so the gauges aren't all zeros.
	s.minimapSubs[ChunkID{X: 1, Y: 1}] = map[uint32]struct{}{1: {}}
	s.subs[ChunkID{X: 0, Y: 0}] = map[uint32]struct{}{1: {}, 2: {}}
	s.chunks[ChunkID{X: 0, Y: 0}] = &ChunkBits{}
	s.minimapMarkDirty(ChunkID{X: 1, Y: 1}, 0)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	s.handleMetrics(w, req)

	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("unexpected Content-Type %q", ct)
	}
	required := []string{
		"go_goroutines",
		"go_memstats_alloc_bytes",
		"go_memstats_sys_bytes",
		"ims_players_connected",
		"ims_chunks_with_state",
		"ims_chunk_subscriptions",
		"ims_minimap_subscriptions",
		"ims_minimap_tiles_allocated",
	}
	for _, name := range required {
		// Each metric should appear with a HELP line and a value line at minimum.
		if !strings.Contains(body, "# TYPE "+name+" ") {
			t.Errorf("metric %s missing TYPE line in output:\n%s", name, body)
		}
		if !strings.Contains(body, name+" ") {
			t.Errorf("metric %s missing value line", name)
		}
	}
	// Spot-check a couple of values reflect server state.
	if !strings.Contains(body, "ims_chunk_subscriptions 2") {
		t.Errorf("expected ims_chunk_subscriptions 2 (matching the 2 subs we set up)")
	}
	if !strings.Contains(body, "ims_minimap_subscriptions 1") {
		t.Errorf("expected ims_minimap_subscriptions 1")
	}
	if !strings.Contains(body, "ims_minimap_tiles_allocated 1") {
		t.Errorf("expected ims_minimap_tiles_allocated 1")
	}
}
