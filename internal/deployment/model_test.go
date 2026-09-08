package deployment

import (
	"strings"
	"testing"
)

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
