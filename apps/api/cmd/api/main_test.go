package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// makeTestCertPEM generates a minimal self-signed certificate, similar to
// the one on the relay proxy (infrastructure/relay-proxy).
func makeTestCertPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "relay.internal"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBuildRelayTLSConfigEmpty(t *testing.T) {
	if cfg := buildRelayTLSConfig(""); cfg != nil {
		t.Fatalf("пустой сертификат должен давать nil (системная проверка), получили %+v", cfg)
	}
	if cfg := buildRelayTLSConfig("   \n"); cfg != nil {
		t.Fatalf("пробельный сертификат должен давать nil, получили %+v", cfg)
	}
}

func TestBuildRelayTLSConfigRawPEM(t *testing.T) {
	pemBytes := makeTestCertPEM(t)
	cfg := buildRelayTLSConfig(string(pemBytes))
	if cfg == nil || cfg.RootCAs == nil {
		t.Fatal("raw PEM должен давать tls.Config с непустым RootCAs")
	}
}

func TestBuildRelayTLSConfigBase64PEM(t *testing.T) {
	pemBytes := makeTestCertPEM(t)
	cfg := buildRelayTLSConfig(base64.StdEncoding.EncodeToString(pemBytes))
	if cfg == nil || cfg.RootCAs == nil {
		t.Fatal("base64(PEM) должен давать tls.Config с непустым RootCAs")
	}
}
