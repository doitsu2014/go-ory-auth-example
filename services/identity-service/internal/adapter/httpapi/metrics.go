package httpapi

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds the RED metrics and the registry served on /metrics.
type Metrics struct {
	Registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewMetrics creates a registry with Go/process collectors and HTTP metrics.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "identity_http_requests_total", Help: "HTTP requests by server, method, route and status.",
		}, []string{"server", "method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "identity_http_request_duration_seconds", Help: "HTTP request duration.",
			Buckets: prometheus.DefBuckets,
		}, []string{"server", "method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration, collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *Metrics) observe(server, method, route string, status int, d time.Duration) {
	m.requests.WithLabelValues(server, method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(server, method, route).Observe(d.Seconds())
}
