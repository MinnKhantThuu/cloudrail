package agent

import (
	"cloudrail/internal/node"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func EnrolledClient(ctx context.Context, dir, bootstrap, url, token string) (*Client, error) {
	keyPath := filepath.Join(dir, "node.key")
	key, e := os.ReadFile(keyPath)
	if errors.Is(e, os.ErrNotExist) {
		_, key, e = node.NewKey()
		if e == nil {
			e = os.WriteFile(keyPath, key, 0600)
		}
	}
	if e != nil {
		return nil, e
	}
	certPath := filepath.Join(dir, "node.crt")
	cert, e := os.ReadFile(certPath)
	if errors.Is(e, os.ErrNotExist) {
		block, _ := pem.Decode(key)
		if block == nil {
			return nil, errors.New("invalid node key")
		}
		private, e := x509.ParseECPrivateKey(block.Bytes)
		if e != nil {
			return nil, e
		}
		der, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "cloudrail-node"}}, private)
		if e != nil {
			return nil, e
		}
		var result struct {
			Certificate string `json:"certificate"`
			CA          string `json:"ca"`
		}
		_, e = NewClient(bootstrap, token).call(ctx, "POST", "/enroll", map[string]string{"csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))}, &result)
		if e != nil {
			return nil, e
		}
		// Persist the CA before the certificate; an interrupted write safely repeats the same CSR.
		if e = os.WriteFile(filepath.Join(dir, "ca.crt"), []byte(result.CA), 0600); e != nil {
			return nil, e
		}
		cert = []byte(result.Certificate)
		if e = os.WriteFile(certPath, cert, 0600); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	ca, e := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid installation CA")
	}
	pair, e := tls.X509KeyPair(cert, key)
	if e != nil {
		return nil, e
	}
	c := NewClient(url, "")
	c.HTTP = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{pair}}}}
	return c, nil
}
func (c *Client) Heartbeat(ctx context.Context, metrics any) error {
	_, e := c.call(ctx, "POST", "/internal/heartbeat", metrics, nil)
	return e
}
