// Package node owns the installation CA and the single trusted execution identity.
package node

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type Authority struct {
	Certificate *x509.Certificate
	Key         *ecdsa.PrivateKey
	PEM         []byte
}

func serial() *big.Int {
	n, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		panic(e)
	}
	return n
}
func encode(kind string, b []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: b})
}
func NewKey() (*ecdsa.PrivateKey, []byte, error) {
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, nil, e
	}
	b, e := x509.MarshalECPrivateKey(k)
	return k, encode("EC PRIVATE KEY", b), e
}
func Load(directory string) (*Authority, error) {
	if e := os.MkdirAll(directory, 0700); e != nil {
		return nil, e
	}
	file := filepath.Join(directory, "authority.pem")
	b, e := os.ReadFile(file)
	if errors.Is(e, os.ErrNotExist) {
		k, key, e := NewKey()
		if e != nil {
			return nil, e
		}
		template := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Cloudrail installation CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(5, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
		der, e := x509.CreateCertificate(rand.Reader, template, template, &k.PublicKey, k)
		if e != nil {
			return nil, e
		}
		b = append(encode("CERTIFICATE", der), key...)
		if e = os.WriteFile(file+".tmp", b, 0600); e != nil {
			return nil, e
		}
		if e = os.Rename(file+".tmp", file); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	certBlock, rest := pem.Decode(b)
	if certBlock == nil {
		return nil, errors.New("invalid CA certificate")
	}
	keyBlock, _ := pem.Decode(rest)
	if keyBlock == nil {
		return nil, errors.New("invalid CA key")
	}
	cert, e := x509.ParseCertificate(certBlock.Bytes)
	if e != nil {
		return nil, e
	}
	key, e := x509.ParseECPrivateKey(keyBlock.Bytes)
	if e != nil {
		return nil, e
	}
	return &Authority{Certificate: cert, Key: key, PEM: encode("CERTIFICATE", cert.Raw)}, nil
}
func (a *Authority) SignCSR(b []byte) (certPEM, serialNumber, hash string, err error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return "", "", "", errors.New("invalid CSR")
	}
	csr, e := x509.ParseCertificateRequest(block.Bytes)
	if e != nil {
		return "", "", "", e
	}
	if e = csr.CheckSignature(); e != nil {
		return "", "", "", e
	}
	if _, ok := csr.PublicKey.(*ecdsa.PublicKey); !ok {
		return "", "", "", errors.New("ECDSA key required")
	}
	n := serial()
	template := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: "cloudrail-node"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, a.Certificate, csr.PublicKey, a.Key)
	if e != nil {
		return "", "", "", e
	}
	h := sha256.Sum256(csr.RawSubjectPublicKeyInfo)
	return string(encode("CERTIFICATE", der)), n.String(), hex.EncodeToString(h[:]), nil
}
func (a *Authority) ServerTLS() (*tls.Config, error) {
	k, key, e := NewKey()
	if e != nil {
		return nil, e
	}
	template := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "server"}, DNSNames: []string{"server", "localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, a.Certificate, &k.PublicKey, a.Key)
	if e != nil {
		return nil, e
	}
	pair, e := tls.X509KeyPair(encode("CERTIFICATE", der), key)
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	pool.AddCert(a.Certificate)
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}, nil
}
