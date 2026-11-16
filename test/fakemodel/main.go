// Command fakemodel serves an OpenAI-compatible embeddings API with fixed
// vectors, for the end-to-end test of 'openarch demo check --semantic' on a
// machine with no model. Each text gets a vector made from a hash of the text:
// the vectors have no meaning, which is enough to check permissions.
//
// Usage: fakemodel <host:port>
package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
)

// dimensions is the size of the vectors. OpenArch pads them to 1024.
const dimensions = 16

func vector(text string) []float32 {
	v := make([]float32, dimensions)
	for i := range v {
		h := fnv.New32a()
		_, _ = fmt.Fprintf(h, "%d:%s", i, text)
		v[i] = float32(h.Sum32()%1000)/1000 + 0.001
	}
	return v
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: fakemodel <host:port>")
	}
	http.HandleFunc("POST /v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(in.Input))
		for i, t := range in.Input {
			data[i] = map[string]any{"object": "embedding", "index": i, "embedding": vector(t)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	log.Fatal(http.ListenAndServe(os.Args[1], nil)) //nolint:gosec // a test helper on a local port
}
