package mcpserver

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Path is the URL path of the Streamable HTTP endpoint.
const Path = "/mcp"

// sessionTimeout closes the sessions of clients that went away without a
// DELETE request.
const sessionTimeout = 30 * time.Minute

// shutdownTimeout is how long open requests may run after the server stops.
const shutdownTimeout = 5 * time.Second

// HTTPOptions configure the Streamable HTTP transport.
type HTTPOptions struct {
	// Token is the bearer token that every request must carry. Empty means no
	// authentication.
	Token string
	// TLS serves HTTPS when it is not nil.
	TLS *tls.Config
}

// Handler returns the HTTP handler of one server for all clients, so that
// the commands that change the workspace still run one at a time. go-sdk
// rejects requests to a loopback address with another Host header (DNS
// rebinding). The handler also rejects browser requests from another origin.
func Handler(opts Options, token string) http.Handler {
	s := New(opts)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{SessionTimeout: sessionTimeout})
	mux := http.NewServeMux()
	mux.Handle(Path, http.NewCrossOriginProtection().Handler(h))
	if token == "" {
		return mux
	}
	return requireToken(token, mux)
}

// requireToken answers 401 to a request without "Authorization: Bearer <token>".
func requireToken(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(strings.TrimSpace(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="openarch"`)
			http.Error(w, "Unauthorized: a valid bearer token is required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ServeHTTP serves the server on l until ctx is done. Then it waits a short
// time for open requests and closes the listener.
func ServeHTTP(ctx context.Context, l net.Listener, opts Options, hopts HTTPOptions) error {
	srv := &http.Server{
		Handler:           Handler(opts, hopts.Token),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if hopts.TLS != nil {
		l = tls.NewListener(l, hopts.TLS)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	// Open event streams never end by themselves, so Shutdown can time out.
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
