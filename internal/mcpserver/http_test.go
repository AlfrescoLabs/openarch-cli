package mcpserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlfrescoLabs/openarch-cli/internal/certs"
)

var upOptions = Options{
	Name: "x", Version: "1",
	Tools: []Tool{{Name: "up", Command: []string{"up"}, Kind: Write}},
	Run: func(_ context.Context, _ []string, progress io.Writer) (int, []byte) {
		_, _ = io.WriteString(progress, "pulling images\nstarting\n")
		return 0, []byte(`{"state": "healthy"}`)
	},
}

// serve starts ServeHTTP on a free loopback port and returns its URL. The
// server stops at the end of the test.
func serve(t *testing.T, opts Options, hopts HTTPOptions) string {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeHTTP(ctx, l, opts, hopts) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("ServeHTTP: %v", err)
		}
	})
	scheme := "http"
	if hopts.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + l.Addr().String() + Path
}

// bearer adds the Authorization header to every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

// connectHTTP connects a client on the protocol version, or on the latest
// version when it is empty.
func connectHTTP(t *testing.T, url string, client *http.Client, copts *mcp.ClientOptions, version string) (*mcp.ClientSession, error) {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, copts).
		Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: client, MaxRetries: -1},
			&mcp.ClientSessionOptions{ProtocolVersion: version})
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// TestHTTPCall calls a tool on the stateless protocol and on the older
// protocol with a handshake. Both get the result and the progress.
func TestHTTPCall(t *testing.T) {
	url := serve(t, upOptions, HTTPOptions{Token: "s3cret"})
	for _, version := range []string{"2026-07-28", "2025-11-25", "2025-06-18"} {
		t.Run(version, func(t *testing.T) { testHTTPCall(t, url, version) })
	}
}

func testHTTPCall(t *testing.T, url, version string) {
	var mu sync.Mutex
	var messages []string
	cs, err := connectHTTP(t, url, &http.Client{Transport: bearer{"s3cret", http.DefaultTransport}},
		&mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, req.Params.Message)
		}}, version)
	if err != nil {
		t.Fatal(err)
	}
	if got := cs.InitializeResult().ProtocolVersion; got != version {
		t.Errorf("protocol version %s, want %s", got, version)
	}
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("tools = %+v, %v", tools, err)
	}
	params := &mcp.CallToolParams{Name: "up", Arguments: map[string]any{}}
	params.SetProgressToken("t1")
	res, err := cs.CallTool(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Content[0].(*mcp.TextContent).Text != `{"state": "healthy"}` {
		t.Errorf("result = %+v", res)
	}
	// The client handles notifications on its own goroutine, so they can arrive after the result.
	deadline := time.Now().Add(5 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	for len(messages) < 2 && time.Now().Before(deadline) {
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
	}
	if !slices.Equal(messages, []string{"pulling images", "starting"}) {
		t.Errorf("progress = %q", messages)
	}
}

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// post sends an initialize request and returns the status and the header of
// the response.
func post(t *testing.T, url string, header map[string]string) (int, http.Header) {
	t.Helper()
	return send(t, http.MethodPost, url, header)
}

// send sends a request with the initialize body and returns the status and
// the header of the response. It reads the whole body, so that the server
// can stop at once.
func send(t *testing.T, method, url string, header map[string]string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, strings.NewReader(initialize))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if h, ok := header["Host"]; ok {
		req.Host = h
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode, res.Header
}

func TestHTTPToken(t *testing.T) {
	url := serve(t, upOptions, HTTPOptions{Token: "s3cret"})
	for _, c := range []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"no token", nil, http.StatusUnauthorized},
		{"wrong token", map[string]string{"Authorization": "Bearer s3cre"}, http.StatusUnauthorized},
		{"basic auth", map[string]string{"Authorization": "Basic czNjcmV0"}, http.StatusUnauthorized},
		{"token", map[string]string{"Authorization": "Bearer s3cret"}, http.StatusOK},
		// DNS rebinding: a loopback server with another Host header.
		{"other host", map[string]string{"Authorization": "Bearer s3cret", "Host": "evil.example"}, http.StatusForbidden},
		{"other origin", map[string]string{"Authorization": "Bearer s3cret", "Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
	} {
		status, header := post(t, url, c.header)
		if status != c.want {
			t.Errorf("%s: status %d, want %d", c.name, status, c.want)
		}
		if id := header.Get("Mcp-Session-Id"); id != "" {
			t.Errorf("%s: session id %q from a stateless server", c.name, id)
		}
		if c.want == http.StatusUnauthorized && !strings.HasPrefix(header.Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("%s: WWW-Authenticate = %q", c.name, header.Get("WWW-Authenticate"))
		}
	}
	if _, err := connectHTTP(t, url, nil, nil, ""); err == nil {
		t.Error("a client without the token connected")
	}
}

func TestHTTPNoToken(t *testing.T) {
	url := serve(t, upOptions, HTTPOptions{})
	if status, _ := post(t, url, nil); status != http.StatusOK {
		t.Errorf("status %d without auth", status)
	}
	if status, _ := post(t, strings.TrimSuffix(url, Path)+"/other", nil); status != http.StatusNotFound {
		t.Errorf("status %d on another path", status)
	}
	// A stateless server has no event stream and no session to delete.
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if status, header := send(t, method, url, map[string]string{"Mcp-Session-Id": "x"}); status != http.StatusMethodNotAllowed || header.Get("Allow") != "POST" {
			t.Errorf("%s: status %d, Allow %q", method, status, header.Get("Allow"))
		}
	}
}

func TestHTTPTLS(t *testing.T) {
	certPEM, keyPEM, err := certs.SelfSigned("localhost", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	url := serve(t, upOptions, HTTPOptions{Token: "s3cret", TLS: &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}})
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	cs, err := connectHTTP(t, url, &http.Client{Transport: bearer{"s3cret", tr}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "up", Arguments: map[string]any{}}); err != nil || res.IsError {
		t.Errorf("call over TLS: %+v, %v", res, err)
	}
}
