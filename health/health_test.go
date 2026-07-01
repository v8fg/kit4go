package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLivenessDefault(t *testing.T) {
	h := New()
	require.True(t, h.Alive())
}

func TestSetAlive(t *testing.T) {
	h := New()
	h.SetAlive(false)
	require.False(t, h.Alive())
	h.SetAlive(true)
	require.True(t, h.Alive())
}

func TestLivenessHandler(t *testing.T) {
	h := New()
	rec := httptest.NewRecorder()
	h.LivenessHandler().ServeHTTP(rec, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	h.SetAlive(false)
	rec = httptest.NewRecorder()
	h.LivenessHandler().ServeHTTP(rec, nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestReadinessNoCheckers(t *testing.T) {
	h := New()
	report := h.IsReady()
	require.Equal(t, StatusHealthy, report.Status)
}

func TestReadinessWithCheckers(t *testing.T) {
	h := New(
		WithChecker(CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}),
		WithChecker(CheckerFunc{CheckerName: "redis", Fn: func() error { return errors.New("ECONNREFUSED") }}),
	)
	report := h.IsReady()
	require.Equal(t, StatusUnhealthy, report.Status)
	require.Len(t, report.Checks, 2)
	require.Equal(t, "db", report.Checks[0].Name)
	require.Equal(t, StatusHealthy, report.Checks[0].Status)
	require.Equal(t, "redis", report.Checks[1].Name)
	require.Equal(t, StatusUnhealthy, report.Checks[1].Status)
	require.Contains(t, report.Checks[1].Error, "ECONNREFUSED")
}

func TestReadinessAllPass(t *testing.T) {
	h := New(
		WithChecker(CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}),
	)
	report := h.IsReady()
	require.Equal(t, StatusHealthy, report.Status)
}

func TestReadinessHandler(t *testing.T) {
	h := New(WithChecker(CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}))
	rec := httptest.NewRecorder()
	h.ReadinessHandler().ServeHTTP(rec, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var report Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	require.Equal(t, StatusHealthy, report.Status)
}

func TestReadinessHandlerUnhealthy(t *testing.T) {
	h := New(WithChecker(CheckerFunc{CheckerName: "db", Fn: func() error { return errors.New("down") }}))
	rec := httptest.NewRecorder()
	h.ReadinessHandler().ServeHTTP(rec, nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var report Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	require.Equal(t, StatusUnhealthy, report.Status)
}

func TestAddCheckerRuntime(t *testing.T) {
	h := New()
	require.Equal(t, StatusHealthy, h.IsReady().Status)
	h.AddChecker(CheckerFunc{CheckerName: "kafka", Fn: func() error { return errors.New("down") }})
	report := h.IsReady()
	require.Equal(t, StatusUnhealthy, report.Status)
	require.Len(t, report.Checks, 1)
}

func TestCacheTTL(t *testing.T) {
	var calls int
	h := New(
		WithChecker(CheckerFunc{
			CheckerName: "db",
			Fn:          func() error { calls++; return nil },
		}),
		WithCacheTTL(50*time.Millisecond),
	)
	h.IsReady() // call 1
	h.IsReady() // cached, no call
	h.IsReady() // cached, no call
	require.Equal(t, 1, calls)
	time.Sleep(60 * time.Millisecond)
	h.IsReady() // cache expired, call 2
	require.Equal(t, 2, calls)
}

func TestReportJSON(t *testing.T) {
	h := New(WithChecker(CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}))
	report := h.IsReady()
	data, err := json.Marshal(report)
	require.NoError(t, err)
	require.Contains(t, string(data), `"status":"healthy"`)
	require.Contains(t, string(data), `"name":"db"`)
}
