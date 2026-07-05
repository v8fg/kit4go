package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/v8fg/kit4go/middleware"
)

func ExampleRequestID() {
	h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(middleware.FromContext(r.Context()) != "")
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	// Output: true
}
