package docker

import (
	"cloudrail/internal/deployment"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRuntimeCommandAndRestartPolicy(t *testing.T) {
	if got := commandOverride("node server.js"); len(got) != 1 || got[0] != "node server.js" {
		t.Fatalf("unexpected command override: %#v", got)
	}
	for _, scenario := range []struct {
		policy string
		max    int
		name   string
		count  int
	}{{"", 0, "on-failure", 10}, {"on-failure", 4, "on-failure", 4}, {"always", 12, "unless-stopped", 0}, {"never", 12, "no", 0}} {
		got := restartPolicy(deployment.Settings{RestartPolicy: scenario.policy, RestartMaxRetries: scenario.max})
		if got["Name"] != scenario.name || got["MaximumRetryCount"] != scenario.count {
			t.Fatalf("policy %#v produced %#v", scenario, got)
		}
	}
}

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
