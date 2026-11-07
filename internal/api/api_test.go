package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/auth"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

const password = "s3cret-Pa55"

// fakeOpenArch answers /v without auth and /api/user with Basic auth admin:s3cret-Pa55.
func fakeOpenArch(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v":
			_, _ = w.Write([]byte(`{"version":"1.0.90"}`))
		case "/api/user":
			if status != 0 {
				w.WriteHeader(status)
				return
			}
			if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"1","username":"admin"}]`))
		default:
			http.NotFound(w, r)
		}
	})
}

func creds(url, pw string) auth.Credentials {
	return auth.Credentials{URL: url, User: "admin", Password: pw}
}

func ping(t *testing.T, c auth.Credentials, log *slog.Logger) (PingResult, error) {
	t.Helper()
	cl, err := New(c, log)
	if err != nil {
		t.Fatal(err)
	}
	return cl.Ping(context.Background())
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(fakeOpenArch(0))
	defer srv.Close()
	res, err := ping(t, creds(srv.URL, password), nil)
	if err != nil || !res.Authenticated || res.Version != "1.0.90" {
		t.Errorf("Ping = %+v, %v", res, err)
	}
}

func TestPingErrors(t *testing.T) {
	srv := httptest.NewServer(fakeOpenArch(0))
	defer srv.Close()
	if _, err := ping(t, creds(srv.URL, "wrong"), nil); output.CodeOf(err) != output.ExitAuth {
		t.Errorf("wrong password: %v", err)
	}
	forbidden := httptest.NewServer(fakeOpenArch(http.StatusForbidden))
	defer forbidden.Close()
	_, err := ping(t, creds(forbidden.URL, password), nil)
	var e *output.Error
	if output.CodeOf(err) != output.ExitAuth || !errors.As(err, &e) || !strings.Contains(e.Message, "no permission") || strings.Contains(e.Hint, "password") {
		t.Errorf("403: %v", err)
	}
	broken := httptest.NewServer(fakeOpenArch(http.StatusInternalServerError))
	defer broken.Close()
	if _, err := ping(t, creds(broken.URL, password), nil); output.CodeOf(err) != output.ExitRemote {
		t.Errorf("500: %v", err)
	}
	broken.Close()
	if _, err := ping(t, creds(broken.URL, password), nil); output.CodeOf(err) != output.ExitRemote {
		t.Errorf("server down: %v", err)
	}
}

// The verbose log must never contain the password, in clear or in Basic auth form.
func TestVerboseLogHasNoPassword(t *testing.T) {
	srv := httptest.NewServer(fakeOpenArch(0))
	defer srv.Close()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if _, err := ping(t, creds(srv.URL, password), log); err != nil {
		t.Fatal(err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte("admin:" + password))
	out := buf.String()
	if strings.Contains(out, password) || strings.Contains(out, basic) {
		t.Errorf("log contains the password:\n%s", out)
	}
	if !strings.Contains(out, "Authorization:REDACTED") || !strings.Contains(out, "status=200") {
		t.Errorf("log does not show the requests:\n%s", out)
	}
}

// A workspace URL can name a host that does not resolve. The client connects
// to DialAddr and verifies the certificate against the workspace certificate.
func TestWorkspaceTLS(t *testing.T) {
	srv := httptest.NewTLSServer(fakeOpenArch(0))
	defer srv.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c := creds("https://example.com", password) // the httptest certificate is valid for example.com
	c.DialAddr = strings.TrimPrefix(srv.URL, "https://")

	_, err := ping(t, c, nil)
	var oerr *output.Error
	if !errors.As(err, &oerr) || oerr.Code != output.ExitRemote || !strings.Contains(oerr.Hint, "not trusted") {
		t.Errorf("untrusted certificate: %v", err)
	}
	c.RootCAPEM = certPEM
	if res, err := ping(t, c, nil); err != nil || !res.Authenticated {
		t.Errorf("trusted workspace certificate: %+v, %v", res, err)
	}
	c.RootCAPEM, c.Insecure = nil, true
	if _, err := ping(t, c, nil); err != nil {
		t.Errorf("--insecure: %v", err)
	}
}
