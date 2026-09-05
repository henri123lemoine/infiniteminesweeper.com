package main

import (
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// processStartTime is set in NewServer; used to expose ims_uptime_seconds.
var processStartTime = time.Now()

// handleMetrics serves a Prometheus text-format snapshot of process and
// app-level gauges. Intended for the dedicated metrics port (see fly.toml's
// [[metrics]] block) so the endpoint is reachable from fly's internal
// scraper but not from the public internet.
//
// Set up alerts in fly-metrics.net Grafana with:
//
//	go_goroutines > 500
//	fly_instance_memory_mem_used > 450 * 1024 * 1024
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s.stateMu.RLock()
	chunks := len(s.chunks)
	chunkSubs := 0
	for _, set := range s.subs {
		chunkSubs += len(set)
	}
	miniSubs := 0
	for _, set := range s.minimapSubs {
		miniSubs += len(set)
	}
	miniTiles := len(s.minimapTiles)
	overviewSubs := 0
	for _, byLOD := range s.overviewSubs {
		overviewSubs += len(byLOD)
	}
	overviewImageBytes := 0
	overviewEncodedBytes := 0
	for _, image := range s.overviewImages {
		overviewImageBytes += len(image.Pixels)
		overviewEncodedBytes += len(image.Encoded)
	}
	overviewTileBytes := len(s.overviewTiles) * (1 + 4 + 16 + 64 + 144 + 256)
	for _, element := range s.overviewDetails {
		detail := element.Value.(*overviewDetail)
		overviewTileBytes += len(detail.full) + len(detail.lod32)
	}
	overviewRequests := s.overviewRequests
	overviewSnapBytes := s.overviewSnapBytes
	overviewPatchBytes := s.overviewPatchBytes
	overviewWireBytes := s.overviewWireBytes
	s.stateMu.RUnlock()

	s.playersMu.RLock()
	players := len(s.players)
	s.playersMu.RUnlock()

	uptime := time.Since(processStartTime).Seconds()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	type m struct {
		name, help, typ string
		value           uint64
	}
	for _, x := range []m{
		{"go_goroutines", "Number of goroutines that currently exist.", "gauge", uint64(runtime.NumGoroutine())},
		{"go_memstats_alloc_bytes", "Number of bytes allocated and still in use.", "gauge", ms.HeapAlloc},
		{"go_memstats_heap_inuse_bytes", "Bytes in in-use heap spans.", "gauge", ms.HeapInuse},
		{"go_memstats_sys_bytes", "Number of bytes obtained from the OS.", "gauge", ms.Sys},
		{"go_memstats_gc_cycles_total", "Number of completed GC cycles.", "counter", uint64(ms.NumGC)},
		{"ims_uptime_seconds", "Process uptime in seconds.", "gauge", uint64(uptime)},
		{"ims_players_connected", "Currently connected players (including spectators).", "gauge", uint64(players)},
		{"ims_chunks_with_state", "Chunks with at least one revealed cell stored.", "gauge", uint64(chunks)},
		{"ims_chunk_subscriptions", "Total (player,chunk) subscription entries for live chunk updates.", "gauge", uint64(chunkSubs)},
		{"ims_minimap_subscriptions", "Total (player,chunk) minimap tile subscriptions.", "gauge", uint64(miniSubs)},
		{"ims_minimap_tiles_allocated", "Allocated minimap tile structs (one per chunk that has been dirty since last unsubscribe).", "gauge", uint64(miniTiles)},
		{"ims_overview_subscriptions", "Active overview image subscriptions by player and LOD.", "gauge", uint64(overviewSubs)},
		{"ims_overview_image_cache_bytes", "Raw bytes in dense global overview images.", "gauge", uint64(overviewImageBytes)},
		{"ims_overview_encoded_cache_bytes", "Compressed bytes in reusable global overview snapshots.", "gauge", uint64(overviewEncodedBytes)},
		{"ims_overview_tile_cache_bytes", "Raw bytes in per-chunk overview pyramids.", "gauge", uint64(overviewTileBytes)},
		{"ims_overview_requests_total", "Overview snapshot requests handled.", "counter", overviewRequests},
		{"ims_overview_snapshot_raw_bytes_total", "Raw overview snapshot pixel bytes sent.", "counter", overviewSnapBytes},
		{"ims_overview_patch_raw_bytes_total", "Raw overview patch pixel bytes sent.", "counter", overviewPatchBytes},
		{"ims_overview_wire_bytes_total", "Compressed overview bytes enqueued to clients.", "counter", overviewWireBytes},
	} {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n", x.name, x.help, x.name, x.typ, x.name, x.value)
	}
}
