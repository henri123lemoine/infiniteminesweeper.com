package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	s := NewServer()
	s.subs[ChunkID{0, 0}] = map[uint32]struct{}{1: {}, 2: {}}
	s.minimapSubs[ChunkID{1, 1}] = map[uint32]struct{}{1: {}}
	s.minimapMarkDirty(ChunkID{1, 1}, 0)

	w := httptest.NewRecorder()
	s.handleMetrics(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()

	for _, want := range []string{
		"go_goroutines ", "go_memstats_sys_bytes ", "ims_players_connected ",
		"ims_chunk_subscriptions 2", "ims_minimap_subscriptions 1", "ims_minimap_tiles_allocated 1",
		"ims_overview_image_cache_bytes ", "ims_overview_requests_total ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in metrics output:\n%s", want, body)
		}
	}
}
