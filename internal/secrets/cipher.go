package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

type Cipher struct{ aead cipher.AEAD }

func New(key string) (*Cipher, error) {
	b, err := hex.DecodeString(key)
	if err != nil || len(b) != 32 {
		return nil, errors.New("CLOUDRAIL_ENCRYPTION_KEY must be 32 random bytes encoded as 64 hex characters")
	}
	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{a}, nil
}
func (c *Cipher) Seal(value, scope string) ([]byte, error) {
	n := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return c.aead.Seal(n, n, []byte(value), []byte(scope)), nil
}
func (c *Cipher) Open(b []byte, scope string) (string, error) {
	n := c.aead.NonceSize()
	if len(b) < n {
		return "", errors.New("invalid encrypted value")
	}
	v, err := c.aead.Open(nil, b[:n], b[n:], []byte(scope))
	return string(v), err
}
