package health_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/v8fg/kit4go/health"
)

// ExampleNew demonstrates building a Health instance with dependency checkers
// and evaluating readiness. With a healthy checker, readiness is healthy; with
// a failing checker, readiness is unhealthy and the failing check surfaces its
// error. Only deterministic fields are printed (the Time field is omitted).
func ExampleNew() {
	// All dependencies healthy.
	h := health.New(
		health.WithChecker(health.CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}),
	)
	report := h.IsReady()
	fmt.Println("ready:", report.Status)
	for _, c := range report.Checks {
		fmt.Println(" -", c.Name, c.Status)
	}

	// One dependency down.
	h.AddChecker(health.CheckerFunc{
		CheckerName: "redis",
		Fn:          func() error { return errors.New("ECONNREFUSED") },
	})
	report = h.IsReady()
	fmt.Println("ready:", report.Status)
	for _, c := range report.Checks {
		fmt.Println(" -", c.Name, c.Status)
	}

	// Output:
	// ready: healthy
	//  - db healthy
	// ready: unhealthy
	//  - db healthy
	//  - redis unhealthy
}

// ExampleHealth_LivenessHandler demonstrates mounting the liveness and
// readiness handlers on an HTTP mux, showing the HTTP status each probe returns
// when the process is alive/dead and when a dependency fails.
func ExampleHealth_LivenessHandler() {
	h := health.New(
		health.WithChecker(health.CheckerFunc{CheckerName: "db", Fn: func() error { return nil }}),
	)

	mux := http.NewServeMux()
	mux.Handle("/healthz", h.LivenessHandler())
	mux.Handle("/readyz", h.ReadinessHandler())

	probe := func(path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	// Healthy process: both probes return 200.
	fmt.Println("alive /healthz:", probe("/healthz"))
	fmt.Println("ready /readyz:", probe("/readyz"))

	// Force a liveness failure (would trigger a pod restart in Kubernetes).
	h.SetAlive(false)
	fmt.Println("dead  /healthz:", probe("/healthz"))

	// Output:
	// alive /healthz: 200
	// ready /readyz: 200
	// dead  /healthz: 503
}
