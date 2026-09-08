package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestWebhookSignature(t *testing.T) {
	body := []byte("Hello, World!")
	signature := "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !Verify("It's a Secret to Everybody", body, signature) {
		t.Fatal("GitHub reference signature rejected")
	}
	if Verify("It's a Secret to Everybody", append(body, '!'), signature) || Verify("", body, signature) {
		t.Fatal("tampering accepted")
	}
}
func TestAppJWT(t *testing.T) {
	k, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	key := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	token, e := JWT("123", key)
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.Split(token, ".")
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if e = rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA256, h[:], sig); e != nil {
		t.Fatal(e)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		ISS string `json:"iss"`
		Exp int64  `json:"exp"`
	}
	if e = json.Unmarshal(payload, &claims); e != nil || claims.ISS != "123" || claims.Exp <= time.Now().Unix() || claims.Exp > time.Now().Add(10*time.Minute).Unix() {
		t.Fatal("invalid JWT claims")
	}
}
