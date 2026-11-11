package certs

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func TestSelfSignedNames(t *testing.T) {
	certPEM, keyPEM, err := SelfSigned("openarch.local", now)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := CheckPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cert.DNSNames, []string{"openarch.local", "localhost"}) {
		t.Errorf("DNSNames = %v", cert.DNSNames)
	}
	if len(cert.IPAddresses) != 2 || !cert.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) || !cert.IPAddresses[1].Equal(net.IPv6loopback) {
		t.Errorf("IPAddresses = %v", cert.IPAddresses)
	}
	for _, h := range []string{"openarch.local", "localhost", "127.0.0.1", "::1"} {
		if err := cert.VerifyHostname(h); err != nil {
			t.Errorf("VerifyHostname(%s): %v", h, err)
		}
	}
	if !strings.Contains(string(keyPEM), "PRIVATE KEY") {
		t.Error("key is not PEM")
	}
}

func TestSelfSignedIPServerName(t *testing.T) {
	dns, ips := Names("192.168.1.10")
	if !slices.Equal(dns, []string{"localhost"}) || len(ips) != 3 {
		t.Errorf("Names = %v, %v", dns, ips)
	}
	dns, ips = Names("localhost")
	if !slices.Equal(dns, []string{"localhost"}) || len(ips) != 2 {
		t.Errorf("Names(localhost) = %v, %v", dns, ips)
	}
}

func TestReusable(t *testing.T) {
	certPEM, keyPEM, err := SelfSigned("a.example", now)
	if err != nil {
		t.Fatal(err)
	}
	if !Reusable(certPEM, keyPEM, "a.example", now) {
		t.Error("fresh certificate not reusable")
	}
	if Reusable(certPEM, keyPEM, "b.example", now) {
		t.Error("reusable for another server name")
	}
	if Reusable(certPEM, keyPEM, "a.example", now.Add(Validity-10*24*time.Hour)) {
		t.Error("reusable 10 days before it expires")
	}
}

func TestCheckPairMismatch(t *testing.T) {
	certPEM, _, err := SelfSigned("a.example", now)
	if err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := SelfSigned("a.example", now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckPair(certPEM, otherKey)
	if err == nil || err.Error() != "the private key does not match the certificate" {
		t.Errorf("CheckPair = %v", err)
	}
	if _, err := CheckPair([]byte("nope"), otherKey); err == nil {
		t.Error("CheckPair accepted a non-PEM certificate")
	}
}

func TestSelfSignedFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	first, _, err := SelfSignedFiles(certFile, keyFile, "localhost", now)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(keyFile); err != nil || fi.Mode().Perm() != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("key file: %v, %v", fi, err)
	}
	again, _, err := SelfSignedFiles(certFile, keyFile, "localhost", now.Add(time.Hour))
	if err != nil || string(again) != string(first) {
		t.Errorf("a reusable pair was not kept: %v", err)
	}
	other, _, err := SelfSignedFiles(certFile, keyFile, "openarch.local", now)
	if err != nil || string(other) == string(first) {
		t.Errorf("a pair for another name was kept: %v", err)
	}
}
