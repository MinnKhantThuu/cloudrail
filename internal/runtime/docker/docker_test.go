package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyReadinessRequiresCorrectRelease(t *testing.T) {
	for _, scenario := range []struct {
		name, header string
		status       int
		pass         bool
	}{{"correct", "new", 200, true}, {"stale route", "old", 200, false}, {"redirect", "new", 302, false}, {"bad health", "new", 503, false}} {
		t.Run(scenario.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != "service.localhost" {
					t.Error("missing service host")
				}
				w.Header().Set("X-Cloudrail-Deployment", scenario.header)
				w.WriteHeader(scenario.status)
			}))
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			err := Check(ctx, s.URL, "service.localhost", "new")
			if (err == nil) != scenario.pass {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}
