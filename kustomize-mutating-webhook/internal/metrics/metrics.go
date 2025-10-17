package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TotalRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mutating_webhook_requests_total",
			Help: "The total number of mutating webhook requests",
		},
		[]string{"resource_kind", "operation"},
	)
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mutating_webhook_request_duration_seconds",
			Help:    "The duration of mutating webhook requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource_kind", "operation"},
	)
	MutationCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mutating_webhook_mutations_total",
			Help: "The total number of mutations performed",
		},
		[]string{"resource_kind"},
	)
	ErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mutating_webhook_errors_total",
			Help: "The total number of errors encountered",
		},
		[]string{"error_type"},
	)
	ConfigReloads = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mutating_webhook_config_reloads_total",
		Help: "The total number of configuration reloads",
	})
	RateLimitedRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mutating_webhook_rate_limited_requests_total",
		Help: "The total number of rate-limited requests",
	})
)
