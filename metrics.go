package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	revealLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "minesweeper",
		Name:      "reveal_latency_microseconds",
		Help:      "Time spent processing a single reveal request",
		Buckets:   prometheus.ExponentialBuckets(10, 2, 10),
	})

	activeConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "minesweeper",
		Name:      "active_connections",
		Help:      "Number of currently connected WebSocket clients",
	})
)

func init() {
	prometheus.MustRegister(revealLatency, activeConnections)
}

// metricsHandler returns the pre‑baked Prometheus HTTP handler
func metricsHandler() http.Handler {
	return promhttp.Handler()
}
