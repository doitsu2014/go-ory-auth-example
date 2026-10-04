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
	// Login identifiers and courier delivery (ADR-0013).
	courier       *prometheus.CounterVec
	preRegister   *prometheus.CounterVec
	unboundLogins prometheus.Gauge
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
	m.courier = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "identity_courier_dispatch_total", Help: "Kratos courier messages by outcome (sent, duplicate, dropped, error), drop reason and channel.",
	}, []string{"outcome", "reason", "channel"})
	m.preRegister = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "identity_login_preregistration_total", Help: "Pre-registration checks by result.",
	}, []string{"result"})
	m.unboundLogins = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "identity_login_unbound_rows", Help: "Unbound login identifier rows (sampled by the purge job).",
	})
	reg.MustRegister(m.courier, m.preRegister, m.unboundLogins)
	reg.MustRegister(m.requests, m.duration, collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *Metrics) observe(server, method, route string, status int, d time.Duration) {
	m.requests.WithLabelValues(server, method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(server, method, route).Observe(d.Seconds())
}

// CourierOutcome counts one courier message.
func (m *Metrics) CourierOutcome(outcome, reason, channel string) {
	if m != nil {
		m.courier.WithLabelValues(outcome, reason, channel).Inc()
	}
}

// PreRegistration counts one pre-registration check.
func (m *Metrics) PreRegistration(result string) {
	if m != nil {
		m.preRegister.WithLabelValues(result).Inc()
	}
}

// UnboundLogins sets the unbound login rows gauge.
func (m *Metrics) UnboundLogins(n int64) {
	if m != nil {
		m.unboundLogins.Set(float64(n))
	}
}
