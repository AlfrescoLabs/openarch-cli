package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func TestEmbeddingType(t *testing.T) {
	for model, want := range map[string]string{
		"ai/mxbai-embed-large":    "ai-mxbai-embed-large",
		"hf.co/Org/Model:Q4_K_M":  "hf.co-org-model-q4_k_m",
		"  spaces//and::colons  ": "spaces-and-colons",
		"already-fine_1.0":        "already-fine_1.0",
	} {
		if got := EmbeddingType(model); got != want {
			t.Errorf("EmbeddingType(%q) = %q, want %q", model, got, want)
		}
	}
}

// OpenArch reads every column by name, so a file without one of them holds no
// embeddings for it. The file must have all five, with the values.
func TestEncodeEmbeddings(t *testing.T) {
	data, err := EncodeEmbeddings("ai-model", []EmbeddingRow{{ID: "0", Text: "Salary bands", Vector: []float32{0.5, -0.25}}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for _, c := range f.Schema().Fields() {
		cols = append(cols, c.Name())
	}
	slices.Sort(cols)
	if !slices.Equal(cols, []string{"blobref", "id", "location", "text", "type", "vector"}) {
		t.Errorf("columns = %v", cols)
	}
	rows, err := parquet.Read[parquetRow](bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "0" || *rows[0].Type != "ai-model" || *rows[0].Text != "Salary bands" ||
		!slices.Equal(rows[0].Vector, []float64{0.5, -0.25}) || rows[0].Location != nil || rows[0].Blobref != nil {
		t.Errorf("rows = %+v", rows)
	}
	if _, err := EncodeEmbeddings("t", []EmbeddingRow{{ID: "big", Vector: make([]float32, VectorDimension+1)}}); err == nil {
		t.Error("a vector over the index size must be refused: OpenArch would store nothing")
	}
}

// fakeEmbeddings records the calls that StoreEmbeddings and SearchEmbeddings make.
type fakeEmbeddings struct {
	childExists bool
	calls       []string
	bodies      map[string]map[string]any
	upload      []byte
	query       map[string]any
}

func (f *fakeEmbeddings) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	call := r.Method + " " + r.URL.Path
	f.calls = append(f.calls, call)
	data, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	f.bodies[call] = body
	switch {
	case call == "POST /api/upload":
		f.upload = data
		if r.URL.Query().Get("mimeType") != "application/x-parquet" || r.Header.Get("Content-Type") != "application/octet-stream" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"up1"}`))
	case call == "GET /api/documents/path/Demo/HR/doc/_e_ai-model":
		if !f.childExists {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"sys_id":"child1","sys_path":"/Demo/HR/doc/_e_ai-model"}`))
	case call == "POST /api/documents/d1":
		if r.URL.Query().Get("enforceSysName") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"sys_id":"child1"}`))
	case r.Method == http.MethodPut:
		_, _ = w.Write([]byte(`{"sys_id":"x"}`))
	case call == "POST /api/query/embeddings":
		f.query = body
		_, _ = w.Write([]byte(`{"embeddings":[{"sysembed_id":"e1","sysembed_docId":"d1","sysembed_score":0.6,"sysembed_text":"Salary bands"}],"totalCount":1}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFakeEmbeddings(t *testing.T, childExists bool) (*fakeEmbeddings, *Client) {
	t.Helper()
	f := &fakeEmbeddings{childExists: childExists, bodies: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(creds(srv.URL, password), nil)
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func TestStoreEmbeddings(t *testing.T) {
	doc := Document{documentSummary: documentSummary{ID: "d1", Path: "/Demo/HR/doc", Mixins: []string{"Other"}}}
	rows := []EmbeddingRow{{ID: "0", Text: "t", Vector: []float32{1}}}

	f, c := newFakeEmbeddings(t, false)
	if err := c.StoreEmbeddings(context.Background(), doc, "ai-model", rows); err != nil {
		t.Fatal(err)
	}
	if mix, _ := json.Marshal(f.bodies["PUT /api/documents/d1"]["sys_mixinTypes"]); string(mix) != `["Other","SysHasEmbeddings"]` {
		t.Errorf("mixins = %s", mix)
	}
	child := f.bodies["POST /api/documents/d1"]
	if child["sys_primaryType"] != "SysEmbeddings" || child["sys_name"] != "_e_ai-model" ||
		child["sysemb_embeddings"].(map[string]any)["uploadId"] != "up1" {
		t.Errorf("child = %v", child)
	}
	if !bytes.HasPrefix(f.upload, []byte("PAR1")) {
		t.Error("the upload is not a Parquet file")
	}

	// Again, on a document that has the mixin and the child: only the file changes.
	doc.Mixins = append(doc.Mixins, "SysHasEmbeddings")
	f, c = newFakeEmbeddings(t, true)
	if err := c.StoreEmbeddings(context.Background(), doc, "ai-model", rows); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.calls, "PUT /api/documents/d1") || slices.Contains(f.calls, "POST /api/documents/d1") {
		t.Errorf("calls = %v", f.calls)
	}
	if b, _ := json.Marshal(f.bodies["PUT /api/documents/child1"]); string(b) != `{"sysemb_embeddings":{"uploadId":"up1"}}` {
		t.Errorf("re-sync body = %s", b)
	}
}

func TestSearchEmbeddings(t *testing.T) {
	f, c := newFakeEmbeddings(t, false)
	hits, total, err := c.SearchEmbeddings(context.Background(), []float32{0.5, 0.5}, "ai-model", "SELECT * FROM SysFile", 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0] != (EmbeddingHit{DocID: "d1", Score: 0.6, Text: "Salary bands"}) {
		t.Errorf("hits = %+v, total %d", hits, total)
	}
	// OpenArch does not pad a query vector, so the CLI does.
	if v := f.query["vector"].([]any); len(v) != VectorDimension || v[0] != 0.5 || v[VectorDimension-1] != 0.0 {
		t.Errorf("vector has %d values", len(v))
	}
	if f.query["embeddingType"] != "ai-model" || f.query["query"] != "SELECT * FROM SysFile" || f.query["limit"] != 3.0 {
		t.Errorf("query = %v", f.query)
	}
	if _, _, err := c.SearchEmbeddings(context.Background(), make([]float32, VectorDimension+1), "t", "", 1); err == nil ||
		!strings.Contains(err.Error(), "at most") {
		t.Errorf("err = %v", err)
	}
}
