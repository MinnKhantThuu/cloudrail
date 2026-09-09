package deployment

import (
	"strings"
	"testing"
)

func TestComputeSpecValidation(t *testing.T) {
	for _, spec := range []ComputeSpec{
		{Name: "api", Environment: "production", WorkloadMode: "web", SourceType: "github"},
		{Name: "queue worker", Environment: "staging", WorkloadMode: "worker", SourceType: "image"},
		{Name: "cleanup", Environment: "production", WorkloadMode: "cron", SourceType: "empty"},
	} {
		if err := spec.Validate(); err != nil {
			t.Fatalf("valid compute spec rejected: %#v: %v", spec, err)
		}
	}
	for _, spec := range []ComputeSpec{
		{Name: "", Environment: "production", WorkloadMode: "web", SourceType: "empty"},
		{Name: "api", Environment: "", WorkloadMode: "web", SourceType: "empty"},
		{Name: "api", Environment: "production", WorkloadMode: "daemon", SourceType: "empty"},
		{Name: "api", Environment: "production", WorkloadMode: "web", SourceType: "gitlab"},
	} {
		if err := spec.Validate(); err == nil {
			t.Fatalf("invalid compute spec accepted: %#v", spec)
		}
	}
}

func TestDeploymentSpecValidation(t *testing.T) {
	good := Spec{Image: "docker.io/traefik/whoami@sha256:" + strings.Repeat("a", 64), Port: 80, HealthPath: "/health"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []Spec{{Image: "nginx:latest", Port: 80, HealthPath: "/"}, {Image: "--privileged@sha256:" + strings.Repeat("a", 64), Port: 80, HealthPath: "/"}, {Image: good.Image, Port: 0, HealthPath: "/"}, {Image: good.Image, Port: 65536, HealthPath: "/"}, {Image: good.Image, Port: 80, HealthPath: "//169.254.169.254"}, {Image: good.Image, Port: 80, HealthPath: "/\r\nHost: evil"}, {Image: good.Image, Port: 80, HealthPath: "http://evil"}}
	for _, s := range tests {
		if s.Validate() == nil {
			t.Errorf("accepted invalid spec: %#v", s)
		}
	}
}

func TestPrivateHostnameIsStableAndDNSCompatible(t *testing.T) {
	got := privateHostname("Checkout API (EU)", "abcdef1234567890")
	if got != "checkout-api-eu-abcdef.internal" {
		t.Fatalf("unexpected private hostname: %q", got)
	}
	service := Service{ID: "abcdef1234567890", Name: "Checkout API (EU)", WorkloadMode: "web", Settings: Settings{Kind: "http"}}
	normalizeServiceSettings(&service)
	if service.Settings.PrivateHost != got || service.Settings.TargetPort != 80 || !service.PublicHTTP() {
		t.Fatalf("legacy service defaults are wrong: %#v", service.Settings)
	}
	disabled := false
	service.Settings.PublicEnabled = &disabled
	if service.PublicHTTP() {
		t.Fatal("internal-only web service was treated as public")
	}
}
