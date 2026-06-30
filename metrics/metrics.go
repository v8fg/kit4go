// Package metrics wraps prometheus/client_golang with opinionated, validated
// builders, an ad-tech-tuned latency histogram, and a ready HTTP exposition
// handler — the one place a service sets up its metrics surface.
//
// Ad-tech use: every stage of the bid pipeline emits metrics (bid QPS, win rate,
// bid/decision latency, error rate, per-SSP breakdown). The latency buckets are
// concentrated in the 1-50ms bidding range so the tail (p99/p999) of the hot
// path is visible without wasting buckets on multi-second ranges.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// LatencyBuckets is the ad-tech-tuned latency histogram in seconds, concentrated
// in the 1-50ms bidding range with light coverage out to multi-second retries.
var LatencyBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// Registry is a prometheus registry plus typed builders. Construct with
// NewRegistry (isolated) or use the package Default (shared).
type Registry struct {
	reg *prometheus.Registry
}

// NewRegistry builds a fresh, isolated registry. Use this in tests and in
// services that want to own their own metric set.
func NewRegistry() *Registry { return &Registry{reg: prometheus.NewRegistry()} }

// defaultRegistry is the package-level shared registry.
var defaultRegistry = NewRegistry()

// Default returns the package-level shared Registry.
func Default() *Registry { return defaultRegistry }

// Prometheus returns the underlying prometheus.Registry for advanced use.
func (r *Registry) Prometheus() *prometheus.Registry { return r.reg }

// Register registers a collector; MustRegister panics on conflict.
func (r *Registry) Register(c prometheus.Collector) error { return r.reg.Register(c) }

// MustRegister registers collectors, panicking on a conflict or invalid metric.
func (r *Registry) MustRegister(cs ...prometheus.Collector) { r.reg.MustRegister(cs...) }

// Gather collects the current metric families (for tests / push gateways).
func (r *Registry) Gather() ([]*dto.MetricFamily, error) { return r.reg.Gather() }

// NewCounter builds and registers a counter vector.
func (r *Registry) NewCounter(name, help string, labels ...string) *prometheus.CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	r.reg.MustRegister(v)
	return v
}

// NewGauge builds and registers a gauge vector.
func (r *Registry) NewGauge(name, help string, labels ...string) *prometheus.GaugeVec {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	r.reg.MustRegister(v)
	return v
}

// NewHistogram builds and registers a histogram vector with the given buckets
// (in seconds for latency, or any unit you document).
func (r *Registry) NewHistogram(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	v := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)
	r.reg.MustRegister(v)
	return v
}

// NewLatencyHistogram builds a latency histogram using LatencyBuckets (seconds,
// ad-tech-tuned). Pair it with kit4go/latency or direct Observe calls.
func (r *Registry) NewLatencyHistogram(name, help string, labels ...string) *prometheus.HistogramVec {
	return r.NewHistogram(name, help, LatencyBuckets, labels...)
}

// Handler returns the HTTP handler exposing the registry in the Prometheus text
// exposition format (mount at /metrics).
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
