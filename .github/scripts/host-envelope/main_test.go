package main

import (
	"bytes"
	"testing"
)

func TestAuthenticatedFixtureTransfer(t *testing.T) {
	key := bytes.Repeat([]byte{42}, 32)
	plain := bytes.Repeat([]byte("private fixture\x00"), 70000)
	sealed, err := transform("seal", key, plain)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := transform("open", key, sealed)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatal("round trip failed")
	}
	other, _ := transform("seal", key, plain)
	if bytes.Equal(other, sealed) {
		t.Fatal("nonce was reused")
	}
	changed := append([]byte{}, sealed...)
	changed[len(changed)-1] ^= 1
	for _, bad := range [][]byte{sealed[:len(sealed)-1], changed, append(sealed, 0), []byte("invalid")} {
		if _, err := transform("open", key, bad); err == nil {
			t.Fatal("damaged envelope accepted")
		}
	}
	if _, err := transform("open", bytes.Repeat([]byte{43}, 32), sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := transform("seal", nil, plain); err == nil {
		t.Fatal("missing key accepted")
	}
}
