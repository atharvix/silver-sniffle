package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	ActiveNearbyUsers   prometheus.Gauge
	OTPSentTotal        *prometheus.CounterVec
	OTPVerifiedTotal    *prometheus.CounterVec
	RateLimitExceeded   *prometheus.CounterVec
	ExternalAPIDuration *prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests processed by method, path, and status code.",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
			},
			[]string{"method", "path"},
		),
		ActiveNearbyUsers: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_nearby_users_count",
				Help: "Current count of active nearby discovery users with fresh heartbeats.",
			},
		),
		OTPSentTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "otp_sent_total",
				Help: "Total OTP codes generated and sent.",
			},
			[]string{"status"},
		),
		OTPVerifiedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "otp_verified_total",
				Help: "Total OTP verification attempts by result status.",
			},
			[]string{"status"},
		),
		RateLimitExceeded: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limit_exceeded_total",
				Help: "Total requests rejected due to rate limiting.",
			},
			[]string{"type", "endpoint"},
		),
		ExternalAPIDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "external_api_duration_seconds",
				Help:    "External API call latency in seconds.",
				Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
			[]string{"service", "operation", "status"},
		),
	}
}
