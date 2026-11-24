// Command rrproxy is a test proxy for the mcp-stateless example. It sends each
// HTTP request to the next upstream in turn (round robin, no sticky sessions)
// and writes one JSON line per request: the upstream, the MCP method, the
// protocol version and any session id. It streams every response, because
// subscriptions/listen stays open. It is a test tool, not a load balancer.
//
// Usage: rrproxy -listen 127.0.0.1:18790 -log wire.jsonl http://127.0.0.1:18791 http://127.0.0.1:18792
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type entry struct {
	Time            string `json:"time"`
	Upstream        string `json:"upstream"`
	HTTPMethod      string `json:"httpMethod"`
	Status          int    `json:"status"`
	MCPMethod       string `json:"mcpMethod,omitempty"`
	MCPName         string `json:"mcpName,omitempty"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	SessionRequest  string `json:"sessionIdRequest,omitempty"`
	SessionResponse string `json:"sessionIdResponse,omitempty"`
}

type ctxKey struct{}

func main() {
	listen := flag.String("listen", "127.0.0.1:18790", "address to listen on")
	logFile := flag.String("log", "", "file for the JSON lines (default stdout)")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: rrproxy [-listen addr] [-log file] upstream-url...")
		os.Exit(2)
	}
	var upstreams []*url.URL
	for _, a := range flag.Args() {
		u, err := url.Parse(a)
		if err != nil || u.Scheme == "" || u.Host == "" {
			log.Fatalf("bad upstream %q", a)
		}
		upstreams = append(upstreams, u)
	}

	out := os.Stdout
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			log.Fatal(err)
		}
		// The file stays open until the process ends.
		out = f
	}
	var mu sync.Mutex
	write := func(e entry) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(out).Encode(e)
	}

	var next atomic.Uint64
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			u := r.In.Context().Value(ctxKey{}).(*url.URL)
			r.SetURL(u)
			r.Out.Host = r.In.Host
		},
		// -1 flushes every write: the notification stream must not wait in a buffer.
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			in := resp.Request
			write(entry{
				Time:            time.Now().Format("15:04:05"),
				Upstream:        in.URL.Host,
				HTTPMethod:      in.Method,
				Status:          resp.StatusCode,
				MCPMethod:       in.Header.Get("Mcp-Method"),
				MCPName:         in.Header.Get("Mcp-Name"),
				ProtocolVersion: in.Header.Get("Mcp-Protocol-Version"),
				SessionRequest:  in.Header.Get("Mcp-Session-Id"),
				SessionResponse: resp.Header.Get("Mcp-Session-Id"),
			})
			return nil
		},
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := upstreams[(next.Add(1)-1)%uint64(len(upstreams))]
		proxy.ServeHTTP(w, r.WithContext(contextWith(r, u)))
	})
	fmt.Fprintf(os.Stderr, "rrproxy on http://%s, %d upstreams\n", *listen, len(upstreams))
	srv := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func contextWith(r *http.Request, u *url.URL) context.Context {
	return context.WithValue(r.Context(), ctxKey{}, u)
}
