package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counts only: never user ids, positions or places. Served at /metrics on the server's local port.
var (
	metricFixes     = promauto.NewCounter(prometheus.CounterOpts{Name: "kinjo_fixes_total", Help: "Positions accepted."})
	metricSightings = promauto.NewCounter(prometheus.CounterOpts{Name: "kinjo_ble_reports_total", Help: "Bluetooth reports received."})
	metricSockets   = promauto.NewGauge(prometheus.GaugeOpts{Name: "kinjo_sockets", Help: "Open phone sockets on this server."})
	metricWakes     = promauto.NewCounterVec(prometheus.CounterOpts{Name: "kinjo_wakes_total", Help: "Wake pushes by result."}, []string{"result"})
	metricRequests  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "kinjo_http_errors_total", Help: "Requests answered with an error, by status."}, []string{"status"})
)
