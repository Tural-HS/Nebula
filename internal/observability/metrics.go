package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds the service's Prometheus collectors on a private registry,
// so tests can create independent instances without global state.
type Metrics struct {
	Registry        *prometheus.Registry
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	JobsTotal       *prometheus.CounterVec
	JobDuration     *prometheus.HistogramVec
}

// NewMetrics creates and registers all collectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nebula_http_requests_total",
			Help: "HTTP requests by route pattern, method and status code.",
		}, []string{"route", "method", "code"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "nebula_http_request_duration_seconds",
			Help:    "HTTP request latency by route pattern and method.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"route", "method"}),
		JobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nebula_jobs_total",
			Help: "Background jobs processed, by kind and result (ok, retry, failed).",
		}, []string{"kind", "result"}),
		JobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "nebula_job_duration_seconds",
			Help:    "Background job run time by kind.",
			Buckets: []float64{.01, .05, .1, .5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"kind"}),
	}
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.RequestsTotal,
		m.RequestDuration,
		m.JobsTotal,
		m.JobDuration,
	)
	return m
}
