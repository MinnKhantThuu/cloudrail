package proxy

import (
	"cloudrail/internal/deployment"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteReplacementAndRemoval(t *testing.T) {
	dir := t.TempDir()
	r := Routes{Directory: dir}
	s := deployment.Service{ID: "service", Host: "service.localhost"}
	for _, id := range []string{"old", "new"} {
		if err := r.Set(s, &deployment.Deployment{ID: id, Port: 8080}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "service.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if json.Unmarshal(b, &parsed) != nil {
			t.Fatal("invalid proxy JSON")
		}
		if !strings.Contains(string(b), "cloudrail-app-"+id+":8080") || !strings.Contains(string(b), "X-Cloudrail-Deployment") {
			t.Fatal(string(b))
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("temporary route files left behind")
	}
	if err := r.Set(s, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(s, nil); err != nil {
		t.Fatal("removal is not idempotent")
	}
}
