package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// JobsTotal is counter of GPU jobs submitted.
	JobsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpu_platform",
		Subsystem: "api",
		Name:      "jobs_total",
		Help:      "Total number of GPU jobs submitted",
	}, []string{"status"})

	// JobsInFlight is gauge of currently running jobs.
	JobsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "gpu_platform",
		Subsystem: "api",
		Name:      "jobs_in_flight",
		Help:      "Number of GPU jobs currently running",
	})

	// JobDurationSeconds is histogram of job duration.
	JobDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "gpu_platform",
		Subsystem: "api",
		Name:      "job_duration_seconds",
		Help:      "Duration of GPU jobs in seconds",
		Buckets:   prometheus.ExponentialBuckets(10, 2, 10),
	}, []string{"status"})

	// APIRequestDuration is histogram of API request latency.
	APIRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "gpu_platform",
		Subsystem: "api",
		Name:      "request_duration_seconds",
		Help:      "API request duration in seconds",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
)
