// Package health provides liveness and readiness health checks for container
// orchestration (Kubernetes probes). Pure standard library.
//
// Liveness: "is the process alive?" (always OK unless deliberately failed).
// Readiness: "can the process serve traffic?" (checks dependencies: DB, Redis,
// downstream SSPs). Both expose an http.HandlerFunc for /healthz and /readyz.
//
// Ad-tech uses: a bidder's readiness depends on its SSP connections, budget
// store, and creative cache being warm. A failed readiness probe removes the pod
// from the load balancer; a failed liveness probe restarts it.
package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Status is the health check result.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

// Checker is a named dependency check. Returns nil if healthy, an error
// describing the failure otherwise.
type Checker interface {
	Name() string
	Check() error
}

// CheckerFunc adapts a function to the Checker interface.
type CheckerFunc struct {
	CheckerName string
	Fn          func() error
}

func (c CheckerFunc) Name() string { return c.CheckerName }
func (c CheckerFunc) Check() error { return c.Fn() }

// CheckerResult holds the outcome of one dependency check.
type CheckerResult struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Report is the full health report (serialized to JSON by the HTTP handler).
type Report struct {
	Status Status          `json:"status"`
	Time   time.Time       `json:"time"`
	Checks []CheckerResult `json:"checks,omitempty"`
}

// Health manages liveness and readiness state with optional dependency checks.
type Health struct {
	mu        sync.RWMutex
	live      atomic.Bool
	checkers  []Checker
	cacheTTL  time.Duration
	cached    *Report
	cachedAt  time.Time
	startTime time.Time
}

// Option configures Health.
type Option func(*Health)

// WithChecker adds a readiness dependency checker.
func WithChecker(c Checker) Option {
	return func(h *Health) { h.checkers = append(h.checkers, c) }
}

// WithCacheTTL caches readiness results for the given duration (avoids hammering
// dependencies on every probe; default 0 = no cache).
func WithCacheTTL(d time.Duration) Option {
	return func(h *Health) { h.cacheTTL = d }
}

// New builds a Health instance. Liveness starts healthy; readiness starts
// unhealthy until all configured checks pass (poll via IsReady).
func New(opts ...Option) *Health {
	h := &Health{startTime: time.Now()}
	h.live.Store(true)
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// SetAlive sets the liveness state. Set false to force a liveness failure
// (triggers pod restart in k8s).
func (h *Health) SetAlive(alive bool) { h.live.Store(alive) }

// Alive reports the current liveness state.
func (h *Health) Alive() bool { return h.live.Load() }

// IsReady evaluates all checkers and returns the readiness report. If cacheTTL
// > 0 and the cached report is fresh, returns the cache without re-running
// checks.
func (h *Health) IsReady() Report {
	if h.cacheTTL > 0 {
		h.mu.RLock()
		if h.cached != nil && time.Since(h.cachedAt) < h.cacheTTL {
			r := *h.cached
			h.mu.RUnlock()
			return r
		}
		h.mu.RUnlock()
	}
	report := h.evaluate()
	if h.cacheTTL > 0 {
		h.mu.Lock()
		h.cached = &report
		h.cachedAt = time.Now()
		h.mu.Unlock()
	}
	return report
}

func (h *Health) evaluate() Report {
	h.mu.RLock()
	checkers := h.checkers
	h.mu.RUnlock()
	report := Report{Time: time.Now(), Status: StatusHealthy}
	for _, c := range checkers {
		err := c.Check()
		result := CheckerResult{Name: c.Name(), Status: StatusHealthy}
		if err != nil {
			result.Status = StatusUnhealthy
			result.Error = err.Error()
			report.Status = StatusUnhealthy
		}
		report.Checks = append(report.Checks, result)
	}
	return report
}

// AddChecker adds a dependency checker at runtime.
func (h *Health) AddChecker(c Checker) {
	h.mu.Lock()
	h.checkers = append(h.checkers, c)
	h.mu.Unlock()
}

// LivenessHandler returns an http.HandlerFunc for /healthz.
func (h *Health) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.live.Load() {
			writeJSON(w, http.StatusServiceUnavailable, Report{
				Status: StatusUnhealthy,
				Time:   time.Now(),
			})
			return
		}
		writeJSON(w, http.StatusOK, Report{
			Status: StatusHealthy,
			Time:   time.Now(),
		})
	}
}

// ReadinessHandler returns an http.HandlerFunc for /readyz.
func (h *Health) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := h.IsReady()
		code := http.StatusOK
		if report.Status != StatusHealthy {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, report)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
