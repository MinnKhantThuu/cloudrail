package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptionBindsScopeAndRejectsTampering(t *testing.T) {
	c, err := New(strings.Repeat("12", 32))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Seal("sensitive-value", "service:one")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("sensitive-value")) {
		t.Fatal("plaintext stored")
	}
	v, err := c.Open(b, "service:one")
	if err != nil || v != "sensitive-value" {
		t.Fatal(err)
	}
	if _, err = c.Open(b, "service:two"); err == nil {
		t.Fatal("scope replay allowed")
	}
	b[len(b)-1] ^= 1
	if _, err = c.Open(b, "service:one"); err == nil {
		t.Fatal("tampering accepted")
	}
}
