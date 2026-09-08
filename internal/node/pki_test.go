package node

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMutualTLSAndPersistentCA(t *testing.T) {
	dir := t.TempDir()
	a, e := Load(dir)
	if e != nil {
		t.Fatal(e)
	}
	again, e := Load(dir)
	if e != nil || string(a.PEM) != string(again.PEM) {
		t.Fatal("CA was not persisted")
	}
	cfg, e := a.ServerTLS()
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	server.TLS = cfg
	server.StartTLS()
	defer server.Close()
	k, key, e := NewKey()
	if e != nil {
		t.Fatal(e)
	}
	csr, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "test"}}, k)
	if e != nil {
		t.Fatal(e)
	}
	cert, _, _, e := a.SignCSR(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))
	if e != nil {
		t.Fatal(e)
	}
	pair, e := tls.X509KeyPair([]byte(cert), key)
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	pool.AddCert(a.Certificate)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "server", MinVersion: tls.VersionTLS13}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	if r, e := client.Get(server.URL); e == nil {
		r.Body.Close()
		t.Fatal("missing client certificate accepted")
	}
	good := transport.Clone()
	good.TLSClientConfig.Certificates = []tls.Certificate{pair}
	defer good.CloseIdleConnections()
	client.Transport = good
	r, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal(r.StatusCode)
	}
}
