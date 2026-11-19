package loader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/auth"
)

// The fingerprint must equal the one the Python preparation scripts compute,
// so a corpus loaded by them is not embedded again. The expected value is
// hashlib.sha256(json.dumps({"model": ..., "passages": ...}, sort_keys=True,
// ensure_ascii=False).encode()).hexdigest() for these passages.
func TestFingerprintMatchesPython(t *testing.T) {
	const passages = `[{"id": "abc:1:2", "engine_id": "31fd285f-5a23-33dc-8e68-139344b10c1a", "text": "Señorías, \"comillas\" \\ barra\ny tab\t y control \u0001   fin — «ñ» 😀 <b>&amp;</b>", "location": {"text": {"page": 13, "paragraph": 12}}}, {"id": "abc:1:3", "text": "Segundo", "location": {"text": {"paragraph": 1, "page": 2}}, "extra": [1, 2.5, true, null]}]`
	dec := json.NewDecoder(strings.NewReader(passages))
	dec.UseNumber()
	var list []map[string]any
	if err := dec.Decode(&list); err != nil {
		t.Fatal(err)
	}
	got, err := Fingerprint("hf.co/gpustack/bge-m3-gguf", list)
	if err != nil {
		t.Fatal(err)
	}
	if want := "aeb628a1e19bd29a51be027907cb32dd723c7e44e34735704eb094159d0aeabd"; got != want {
		t.Errorf("fingerprint = %s, want %s", got, want)
	}
}

// engine is an in-memory OpenArch with the calls the loader makes.
type engine struct {
	mu      sync.Mutex
	docs    map[string]map[string]any // by path
	byID    map[string]string         // id to path
	uploads map[string][]byte
	model   map[string]any
	patches []string
	calls   []string
	next    int
}

func newEngine() *engine {
	e := &engine{docs: map[string]map[string]any{}, byID: map[string]string{}, uploads: map[string][]byte{},
		model: map[string]any{"schemas": map[string]any{}, "mixinTypes": map[string]any{}}}
	e.put("/", map[string]any{"sys_primaryType": "SysFolder"})
	return e
}

func (e *engine) put(path string, props map[string]any) map[string]any {
	e.next++
	id := fmt.Sprintf("id%d", e.next)
	props["sys_id"], props["sys_path"] = id, path
	e.docs[path], e.byID[id] = props, path
	return props
}

func (e *engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	data, _ := io.ReadAll(r.Body)
	e.calls = append(e.calls, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/documents/path/"):
		p, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/documents/path"))
		if d, ok := e.docs[p]; ok {
			out := map[string]any{}
			for k, v := range d {
				if k != "sys_fulltextBinary" { // OpenArch never returns it
					out[k] = v
				}
			}
			write(out)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		write(map[string]string{"message": "not found"})
	case r.Method == http.MethodPost && r.URL.Path == "/api/upload":
		e.next++
		id := fmt.Sprintf("up%d", e.next)
		e.uploads[id] = data
		write(map[string]string{"id": id})
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/documents/"):
		parent, ok := e.byID[strings.TrimPrefix(r.URL.Path, "/api/documents/")]
		if !ok || r.URL.Query().Get("enforceSysName") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		e.blob(body)
		write(e.put(strings.TrimSuffix(parent, "/")+"/"+body["sys_name"].(string), body))
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/documents/"):
		d := e.docs[e.byID[strings.TrimPrefix(r.URL.Path, "/api/documents/")]]
		e.blob(body)
		for k, v := range body {
			d[k] = v
		}
		write(d)
	case r.URL.Path == "/api/repository/model" && r.Method == http.MethodGet:
		write(e.model)
	case r.URL.Path == "/api/repository/model" && r.Method == http.MethodPatch:
		e.patches = append(e.patches, r.URL.Query().Get("validateOnly")+" "+string(data))
		if r.URL.Query().Get("validateOnly") == "false" {
			var ops []struct {
				Path  string
				Value any
			}
			_ = json.Unmarshal(data, &ops)
			for _, op := range ops {
				parts := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
				m := e.model
				for _, p := range parts[:len(parts)-1] {
					m = m[p].(map[string]any)
				}
				m[parts[len(parts)-1]] = op.Value
			}
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// blob turns an upload id into a stored blob with the digest OpenArch reports.
func (e *engine) blob(body map[string]any) {
	for _, k := range []string{"sysfile_blob", "sysemb_embeddings"} {
		if b, ok := body[k].(map[string]any); ok {
			sum := sha256.Sum256(e.uploads[b["uploadId"].(string)])
			body[k] = map[string]any{"digest": hex.EncodeToString(sum[:]), "upload": b["uploadId"]}
		}
	}
}

func (e *engine) count(prefix string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, c := range e.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func client(t *testing.T, e *engine) *api.Client {
	t.Helper()
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	c, err := api.New(auth.Credentials{URL: srv.URL, User: "admin", Password: "pw"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const original = "%PDF-1.7 session record"

// setup writes an original file and a plan for it, and returns the options.
func setup(t *testing.T, metadata string) (Options, string, *int) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "s.pdf"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(original))
	hash := hex.EncodeToString(sum[:])
	plan := fmt.Sprintf(`{"original": {"path": "/corpus/sources/s.pdf", "file": "s.pdf",
	  "metadata": {"fc_sourceSha256": %q, "fc_date": "2020-01-04T00:00:00.000Z"}},
	 "documents": [{"path": "/corpus/s/turn-0001", "metadata": {"fc_sourceSha256": %q, "fc_turn": 1%s},
	   "passages": [{"id": "s:1:1", "text": "Señorías, primera.", "location": {"text": {"page": 3, "paragraph": 1}}},
	                {"id": "s:1:2", "text": "Segunda.", "location": {"text": {"page": 3, "paragraph": 2}}}]}],
	 "stats": {"turns": 1}}`, hash, hash, metadata)
	file := filepath.Join(root, "plans", "s.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	embedded := new(int)
	o := Options{Model: "m", EmbeddingType: "m", Batch: 1, MaxPassageBytes: 8000, Mixins: []string{"FcSource"},
		HashField: "fc_sourceSha256", Root: root, Cache: filepath.Join(root, "cache"), Done: filepath.Join(root, "done"), Workers: 2,
		Schema: []byte(`{"schemas": {"factcheck": {"prefix": "fc", "fields": {"fc_turn": {"type": "long"}}}},
		                 "mixinTypes": {"FcSource": {"schemas": ["factcheck"]}}}`),
		Embed: func(_ context.Context, texts []string) ([][]float32, error) {
			*embedded += len(texts)
			out := make([][]float32, len(texts))
			for i := range texts {
				out[i] = []float32{1, float32(i)}
			}
			return out, nil
		}}
	return o, file, embedded
}

func TestLoadThenLoadAgain(t *testing.T) {
	e := newEngine()
	c := client(t, e)
	o, file, embedded := setup(t, "")
	res, err := Load(context.Background(), c, []string{file}, o)
	if err != nil || len(res.Errors) > 0 {
		t.Fatal(err, res.Errors)
	}
	if res.Loaded != 1 || res.Documents != 1 || res.Passages != 2 || res.Created != 2 || res.EmbeddingsStored != 1 || res.ModelOperations != 2 {
		t.Errorf("first load = %+v", res)
	}
	if *embedded != 3 { // the longest passage once, then both passages
		t.Errorf("embedded %d texts, want 3", *embedded)
	}
	doc := e.docs["/corpus/s/turn-0001"]
	if doc["sys_primaryType"] != "SysFile" || doc["sys_title"] != "turn-0001" || doc["sys_fulltextBinary"] != "Señorías, primera.\n\nSegunda." {
		t.Errorf("document = %v", doc)
	}
	if m := fmt.Sprint(doc["sys_mixinTypes"]); m != "[FcSource SysHasEmbeddings]" {
		t.Errorf("mixins = %s", m)
	}
	if e.docs["/corpus/sources"]["sys_primaryType"] != "SysFolder" {
		t.Error("the parent folders are created")
	}
	child := e.docs["/corpus/s/turn-0001/_e_m"]
	file1 := e.uploads[child["sysemb_embeddings"].(map[string]any)["upload"].(string)]
	rows, err := parquet.Read[struct {
		ID       string  `parquet:"id"`
		Location *string `parquet:"location,optional"`
	}](bytes.NewReader(file1), int64(len(file1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].ID != "s:1:2" || *rows[1].Location != `{"text": {"page": 3, "paragraph": 2}}` {
		t.Errorf("rows = %+v", rows)
	}

	// A marker skips the plan; --all loads it again and finds nothing to change.
	res, err = Load(context.Background(), c, []string{file}, o)
	if err != nil || res.Skipped != 1 || res.Loaded != 0 {
		t.Fatalf("with a marker: %+v, %v", res, err)
	}
	uploads, embeddedBefore := e.count("POST /api/upload"), *embedded
	o.All = true
	res, err = Load(context.Background(), c, []string{file}, o)
	if err != nil || len(res.Errors) > 0 {
		t.Fatal(err, res.Errors)
	}
	if res.Unchanged != 2 || res.EmbeddingsUnchanged != 1 || res.ModelOperations != 0 || e.count("POST /api/upload") != uploads || e.count("PUT ") != 0 {
		t.Errorf("second load changed something: %+v", res)
	}
	if *embedded != embeddedBefore+1 { // only the check with the longest passage
		t.Errorf("embedded %d texts again", *embedded-embeddedBefore)
	}
}

func TestLoadNewMetadataKeepsFileAndUsesCache(t *testing.T) {
	e := newEngine()
	c := client(t, e)
	o, file, embedded := setup(t, "")
	if _, err := Load(context.Background(), c, []string{file}, o); err != nil {
		t.Fatal(err)
	}
	// A new field: the document gets the metadata only, the embeddings stay.
	o2, file2, _ := setup(t, `, "fc_office": "Presidente"`)
	o2.Cache, o2.All = o.Cache, true
	o2.Embed = o.Embed
	before := *embedded
	res, err := Load(context.Background(), c, []string{file2}, o2)
	if err != nil || len(res.Errors) > 0 {
		t.Fatal(err, res.Errors)
	}
	if res.MetadataOnly != 1 || res.Uploaded != 0 || res.EmbeddingsUnchanged != 1 || e.docs["/corpus/s/turn-0001"]["fc_office"] != "Presidente" {
		t.Errorf("metadata change = %+v", res)
	}
	// In a new repository the embeddings come from the cache.
	e2 := newEngine()
	res, err = Load(context.Background(), client(t, e2), []string{file2}, o2)
	if err != nil || len(res.Errors) > 0 {
		t.Fatal(err, res.Errors)
	}
	if res.EmbeddingsStored != 1 || res.Embedded != 0 || *embedded != before+2 { // two preflights only
		t.Errorf("cache not used: %+v, %d", res, *embedded-before)
	}
}

func TestLoadRefusesChangedSource(t *testing.T) {
	e := newEngine()
	c := client(t, e)
	o, file, _ := setup(t, "")
	if _, err := Load(context.Background(), c, []string{file}, o); err != nil {
		t.Fatal(err)
	}
	e.docs["/corpus/s/turn-0001"]["fc_sourceSha256"] = "other"
	o.All = true
	res, err := Load(context.Background(), c, []string{file}, o)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Errors["s.json"], "not overwritten") || res.Loaded != 0 {
		t.Errorf("result = %+v", res)
	}
	// The original file itself must match the hash in the plan.
	if err := os.WriteFile(filepath.Join(o.Root, "s.pdf"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = Load(context.Background(), c, []string{file}, o)
	if !strings.Contains(res.Errors["s.json"], "changed after the plan was made") {
		t.Errorf("result = %+v", res)
	}
}

func TestPreflightFailsBeforeAnyChange(t *testing.T) {
	e := newEngine()
	c := client(t, e)
	o, file, _ := setup(t, "")
	o.Embed = func(context.Context, []string) ([][]float32, error) { return nil, errors.New("input too large") }
	if _, err := Load(context.Background(), c, []string{file}, o); err == nil || !strings.Contains(err.Error(), "longest passage") {
		t.Fatalf("err = %v", err)
	}
	if len(e.calls) != 0 {
		t.Errorf("calls before the check: %v", e.calls)
	}
	o, file, _ = setup(t, "")
	o.MaxPassageBytes = 10
	if _, err := Load(context.Background(), c, []string{file}, o); err == nil || !strings.Contains(err.Error(), "split it") {
		t.Errorf("a long passage must fail: %v", err)
	}
}

func TestEnsureModelIsAddOnly(t *testing.T) {
	e := newEngine()
	c := client(t, e)
	e.model["schemas"] = map[string]any{"factcheck": map[string]any{"prefix": "fc", "fields": map[string]any{"fc_turn": map[string]any{"type": "long"}}}}
	n, err := EnsureModel(context.Background(), c, []byte(`{"schemas": {"factcheck": {"prefix": "fc",
	  "fields": {"fc_turn": {"type": "long"}, "fc_office": {"type": "string"}}}}}`))
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if len(e.patches) != 2 || !strings.HasPrefix(e.patches[0], "true ") || !strings.Contains(e.patches[1], `"/schemas/factcheck/fields/fc_office"`) {
		t.Errorf("patches = %v", e.patches)
	}
	if _, err := EnsureModel(context.Background(), c, []byte(`{"schemas": {"factcheck": {"prefix": "fc",
	  "fields": {"fc_turn": {"type": "string"}}}}}`)); err == nil || !strings.Contains(err.Error(), "cannot change") && !strings.Contains(err.Error(), "is of type") {
		t.Errorf("a changed field type must fail: %v", err)
	}
}

func TestEqual(t *testing.T) {
	for _, c := range []struct {
		a, b any
		want bool
	}{
		{"2020-01-04T00:00:00Z", "2020-01-04T00:00:00.000Z", true},
		{"2020-01-04T00:00:00Z", "2020-01-05T00:00:00.000Z", false},
		{json.Number("14"), json.Number("14.0"), true},
		{json.Number("14"), "14", false},
		{nil, "x", false},
		{[]any{"a"}, []any{"a"}, true},
	} {
		if got := equal(c.a, c.b); got != c.want {
			t.Errorf("equal(%v, %v) = %v", c.a, c.b, got)
		}
	}
}
