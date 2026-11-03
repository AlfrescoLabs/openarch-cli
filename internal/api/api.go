// Package api is the OpenArch REST client. The generated code is in package
// gen (run 'make generate' after a change to api/openapi.json).
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api/gen"
	"github.com/AlfrescoLabs/openarch-cli/internal/auth"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Client calls the OpenArch API with one set of credentials.
type Client struct {
	creds auth.Credentials
	http  *http.Client
	Gen   *gen.ClientWithResponses
}

// New returns a client. log receives one debug line per request and response,
// with the Authorization header redacted.
func New(creds auth.Credentials, log *slog.Logger) (*Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: creds.Insecure} //nolint:gosec // --insecure is the user's explicit choice, with a warning.
	if len(creds.RootCAPEM) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		pool.AppendCertsFromPEM(creds.RootCAPEM)
		tlsConfig.RootCAs = pool
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if creds.DialAddr != "" {
		addr := creds.DialAddr
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		}
	}
	hc := &http.Client{
		Timeout:   60 * time.Second,
		Transport: &authTransport{next: transport, user: creds.User, password: creds.Password, log: log},
	}
	g, err := gen.NewClientWithResponses(creds.URL, gen.WithHTTPClient(hc))
	if err != nil {
		return nil, err
	}
	return &Client{creds: creds, http: hc, Gen: g}, nil
}

// authTransport adds HTTP Basic auth and logs requests without secrets.
type authTransport struct {
	next     http.RoundTripper
	user     string
	password string
	log      *slog.Logger
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.user, t.password)
	start := time.Now()
	if t.log != nil {
		t.log.Debug("http request", "method", req.Method, "url", req.URL.String(), "headers", redact(req.Header))
	}
	resp, err := t.next.RoundTrip(req)
	if t.log != nil {
		if err != nil {
			t.log.Debug("http error", "method", req.Method, "url", req.URL.String(), "error", err.Error())
		} else {
			t.log.Debug("http response", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode, "duration", time.Since(start).Round(time.Millisecond).String())
		}
	}
	return resp, err
}

// redact returns the headers with secret values replaced.
func redact(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		switch http.CanonicalHeaderKey(k) {
		case "Authorization", "Cookie", "Proxy-Authorization":
			out[k] = "REDACTED"
		default:
			if len(v) > 0 {
				out[k] = v[0]
			}
		}
	}
	return out
}

// Check maps an HTTP status to a CLI error. 401 and 403 are authentication
// errors (exit code 6); other non-2xx statuses are remote errors (exit code 5).
func (c *Client) Check(status int, what string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return output.NewError(output.ExitAuth,
			fmt.Sprintf("%s: authentication failed at %s (HTTP %d)", what, c.creds.URL, status),
			"Credentials used: "+c.creds.Describe()+".")
	default:
		return output.NewError(output.ExitRemote, fmt.Sprintf("%s: HTTP %d from %s", what, status, c.creds.URL), "")
	}
}

// Transport turns a request error into a CLI error with exit code 5.
func (c *Client) Transport(err error, what string) error {
	var uerr *url.Error
	msg := err.Error()
	if errors.As(err, &uerr) {
		msg = uerr.Err.Error()
	}
	hint := "Check the URL and the network."
	if c.creds.Workspace != "" {
		hint = "Is the stack running? Run 'openarch status'."
	}
	var cerr *tls.CertificateVerificationError
	if errors.As(err, &cerr) {
		hint = "The server certificate is not trusted. Add it to the system trust store, or use --insecure."
	}
	return output.NewError(output.ExitRemote, fmt.Sprintf("%s: cannot reach %s: %s", what, c.creds.URL, msg), hint)
}

// PingResult is the outcome of Ping.
type PingResult struct {
	URL           string       `json:"url"`
	User          string       `json:"user"`
	Version       string       `json:"version,omitempty"`
	Authenticated bool         `json:"authenticated"`
	Sources       auth.Sources `json:"sources"`
	Workspace     string       `json:"workspace,omitempty"`
}

// Ping reads the server version and checks the credentials with an authenticated call.
func (c *Client) Ping(ctx context.Context) (PingResult, error) {
	res := PingResult{URL: c.creds.URL, User: c.creds.User, Sources: c.creds.Sources, Workspace: c.creds.Workspace}
	res.Version = c.version(ctx)
	resp, err := c.Gen.SearchUsersByNameWithResponse(ctx, &gen.SearchUsersByNameParams{Name: c.creds.User})
	if err != nil {
		return res, c.Transport(err, "ping")
	}
	if err := c.Check(resp.StatusCode(), "ping"); err != nil {
		return res, err
	}
	res.Authenticated = true
	return res, nil
}

// version reads GET /v, which needs no authentication. It returns "" on any problem.
func (c *Client) version(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.creds.URL+"/v", nil)
	if err != nil {
		return ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var v struct {
		Version string `json:"version"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&v) != nil {
		return ""
	}
	return v.Version
}

// WriteText writes the human-readable form.
func (p PingResult) WriteText(w io.Writer) error {
	version := p.Version
	if version == "" {
		version = "unknown"
	}
	_, err := fmt.Fprintf(w, "OpenArch %s at %s: authenticated as %s\n", version, p.URL, p.User)
	return err
}
