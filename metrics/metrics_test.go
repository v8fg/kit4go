package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/metrics"
)

func TestCounterBuildsAndCounts(t *testing.T) {
	reg := metrics.NewRegistry()
	c := reg.NewCounter("bids_total", "total bid requests", "ssp")
	c.WithLabelValues("a").Add(1)
	c.WithLabelValues("a").Add(2)
	c.WithLabelValues("b").Add(5)

	// ToFloat64 requires exactly one sample, so pass the labeled counter.
	require.InDelta(t, 3.0, testutil.ToFloat64(c.WithLabelValues("a")), 1e-9)
	require.InDelta(t, 5.0, testutil.ToFloat64(c.WithLabelValues("b")), 1e-9)
}

func TestGauge(t *testing.T) {
	reg := metrics.NewRegistry()
	g := reg.NewGauge("budget_remaining", "remaining budget", "campaign")
	g.WithLabelValues("x").Set(100)
	g.WithLabelValues("x").Sub(30)
	require.InDelta(t, 70.0, testutil.ToFloat64(g.WithLabelValues("x")), 1e-9)
}

func TestHistogramObserve(t *testing.T) {
	reg := metrics.NewRegistry()
	h := reg.NewHistogram("bid_latency_seconds", "bid decision latency",
		[]float64{0.01, 0.05, 0.1}, "ssp")
	h.WithLabelValues("a").Observe(0.02)
	h.WithLabelValues("a").Observe(0.2)
	// One histogram series ("a") collected.
	require.Equal(t, 1, int(testutil.CollectAndCount(h)))
}

func TestLatencyHistogramUsesTunedBuckets(t *testing.T) {
	reg := metrics.NewRegistry()
	h := reg.NewLatencyHistogram("lat", "latency", "route")
	h.WithLabelValues("r") // instantiate a child so the histogram is collected
	// Collecting the histogram exposes the configured buckets in the text format.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	require.Contains(t, body, "lat_bucket")
	require.Contains(t, body, `le="0.001"`) // 1ms bucket present
}

func TestHandlerExposesMetrics(t *testing.T) {
	reg := metrics.NewRegistry()
	reg.NewCounter("foo_total", "foo", "k").WithLabelValues("v").Add(7)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "foo_total")
	require.True(t, strings.Contains(rec.Body.String(), " 7"))
}

func TestGatherReturnsFamilies(t *testing.T) {
	reg := metrics.NewRegistry()
	c := reg.NewCounter("c_total", "c")
	c.WithLabelValues() // no-label vec: instantiate the single child so it's collected
	fams, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, fams, 1)
	require.Equal(t, "c_total", fams[0].GetName())
}

func TestDefaultRegistryIsShared(t *testing.T) {
	require.Same(t, metrics.Default(), metrics.Default())
}

func TestLatencyBucketsFocusedOnBiddingRange(t *testing.T) {
	// Buckets must include sub-50ms values (the bidding hot path).
	hasSub50ms := false
	for _, b := range metrics.LatencyBuckets {
		if b < 0.05 {
			hasSub50ms = true
		}
	}
	require.True(t, hasSub50ms, "latency buckets should cover the <50ms bidding range")
	require.NotEmpty(t, metrics.LatencyBuckets)
}

func TestRegisterReturnsErrorOnDuplicate(t *testing.T) {
	reg := metrics.NewRegistry()
	c := reg.NewCounter("dup_total", "dup")
	// Registering the same collector again must error (AlreadyRegistered).
	err := reg.Register(c)
	require.Error(t, err)
}

func TestMustRegister(t *testing.T) {
	reg := metrics.NewRegistry()
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "mr_total", Help: "mr"})
	require.NotPanics(t, func() { reg.MustRegister(c) })
}

func TestPrometheusUnderlying(t *testing.T) {
	reg := metrics.NewRegistry()
	require.NotNil(t, reg.Prometheus())
}
