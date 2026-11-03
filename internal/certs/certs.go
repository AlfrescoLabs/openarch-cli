// Package certs creates self-signed certificates and checks certificate files.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"slices"
	"strings"
	"time"
)

// Validity of self-signed certificates. 825 days is the longest that macOS and iOS accept.
const Validity = 825 * 24 * time.Hour

// Names returns the DNS names and IP addresses a self-signed certificate covers for serverName.
func Names(serverName string) (dns []string, ips []net.IP) {
	add := func(h string) {
		if ip := net.ParseIP(h); ip != nil {
			if !slices.ContainsFunc(ips, ip.Equal) {
				ips = append(ips, ip)
			}
		} else if !slices.Contains(dns, h) {
			dns = append(dns, h)
		}
	}
	for _, h := range []string{serverName, "localhost", "127.0.0.1", "::1"} {
		add(h)
	}
	return dns, ips
}

// SelfSigned returns a new self-signed certificate and its private key, both PEM encoded.
func SelfSigned(serverName string, now time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	dns, ips := Names(serverName)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: serverName, Organization: []string{"OpenArch CLI self-signed"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(Validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dns,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// Reusable reports whether an existing self-signed certificate can be kept:
// the pair is valid, it covers serverName and it does not expire within 30 days.
func Reusable(certPEM, keyPEM []byte, serverName string, now time.Time) bool {
	cert, err := CheckPair(certPEM, keyPEM)
	if err != nil {
		return false
	}
	if now.Add(30*24*time.Hour).After(cert.NotAfter) || now.Before(cert.NotBefore) {
		return false
	}
	return cert.VerifyHostname(serverName) == nil
}

// CheckPair parses a certificate and its private key and checks that they match.
// It returns the first certificate of the chain.
func CheckPair(certPEM, keyPEM []byte) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, describe(err)
	}
	return x509.ParseCertificate(pair.Certificate[0])
}

func describe(err error) error {
	msg := err.Error()
	if msg == "tls: private key does not match public key" {
		return errors.New("the private key does not match the certificate")
	}
	if m, ok := strings.CutPrefix(msg, "tls: "); ok {
		return errors.New(m)
	}
	return fmt.Errorf("invalid certificate or key: %w", err)
}
