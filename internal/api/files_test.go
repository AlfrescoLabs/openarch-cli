package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// fakeFiles is an in-memory OpenArch with one SysFile document, "d1", under the root.
type fakeFiles struct {
	uploads     map[string][]byte
	blob        []byte
	name        string
	mimeType    string
	failStep    string // "upload" or "attach": answer 500 at that step
	corrupt     bool   // serve other bytes than the stored digest
	deleted     bool
	lastQuery   map[string]any
	querySync   string
	description string
}

func (f *fakeFiles) doc() []byte {
	d := map[string]any{"sys_id": "d1", "sys_primaryType": "SysFile", "sys_name": f.name, "sys_path": "/" + f.name}
	if f.blob != nil {
		sum := sha256.Sum256(f.blob)
		d["sysfile_blob"] = map[string]any{"filename": f.name, "mimeType": f.mimeType, "length": len(f.blob), "digest": hex.EncodeToString(sum[:])}
	}
	data, _ := json.Marshal(d)
	return data
}

func (f *fakeFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/documents/path/":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.name = body["sys_name"]
		_, _ = w.Write(f.doc())
	case r.Method == http.MethodPost && r.URL.Path == "/api/upload":
		if f.failStep == "upload" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		data, _ := io.ReadAll(r.Body)
		f.uploads["up1"] = data
		f.mimeType = r.URL.Query().Get("mimeType")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"up1","status":"READY"}`))
	case r.Method == http.MethodPut && r.URL.Path == "/api/documents/d1":
		if f.failStep == "attach" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var body struct {
			Blob *struct {
				UploadID string `json:"uploadId"`
			} `json:"sysfile_blob"`
			Description *string `json:"sys_description"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Blob != nil {
			f.blob = f.uploads[body.Blob.UploadID]
		}
		if body.Description != nil {
			f.description = *body.Description
		}
		_, _ = w.Write(f.doc())
	case r.Method == http.MethodGet && r.URL.Path == "/api/documents/path/":
		_, _ = w.Write([]byte(`{"sys_id":"root","sys_primaryType":"SysRoot","sys_path":"/"}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/api/documents/d1":
		f.deleted = true
	case r.Method == http.MethodGet && f.name != "" && (r.URL.Path == "/api/documents/d1" || r.URL.Path == "/api/documents/path/"+f.name):
		_, _ = w.Write(f.doc())
	case r.Method == http.MethodGet && r.URL.Path == "/api/download/d1/sysfile_blob/"+f.name:
		w.Header().Set("Content-Type", f.mimeType)
		if f.corrupt {
			_, _ = w.Write([]byte("other bytes"))
			return
		}
		_, _ = w.Write(f.blob)
	case r.Method == http.MethodPost && r.URL.Path == "/api/query":
		f.querySync = r.URL.Query().Get("sync")
		_ = json.NewDecoder(r.Body).Decode(&f.lastQuery)
		_, _ = w.Write([]byte(`{"documents":[` + string(f.doc()) + `],"count":1,"totalCount":1,"offset":0}`))
	default:
		http.NotFound(w, r)
	}
}

func filesClient(t *testing.T, f *fakeFiles) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(creds(srv.URL, password), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUploadThenDownload(t *testing.T) {
	f := &fakeFiles{uploads: map[string][]byte{}}
	c := filesClient(t, f)
	content := []byte("%PDF-1.4 a quote for a charging point")
	doc, err := c.UploadFile(context.Background(), "/", "quote.pdf", "Quote", "", "application/pdf", bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != "d1" || doc.Blob == nil || doc.Blob.Length != int64(len(content)) || f.mimeType != "application/pdf" {
		t.Fatalf("doc = %+v, blob %+v, mime %q", doc, doc.Blob, f.mimeType)
	}
	var out bytes.Buffer
	res, err := c.Download(context.Background(), "/quote.pdf", &out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if !bytes.Equal(out.Bytes(), content) || res.SHA256 != hex.EncodeToString(sum[:]) || res.Length != int64(len(content)) {
		t.Errorf("download = %q, %+v", out.String(), res)
	}
}

func TestDownloadChecksDigest(t *testing.T) {
	f := &fakeFiles{uploads: map[string][]byte{}, corrupt: true}
	c := filesClient(t, f)
	if _, err := c.UploadFile(context.Background(), "/", "a.txt", "a", "", "text/plain", strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	_, err := c.Download(context.Background(), "d1", io.Discard)
	if output.CodeOf(err) != output.ExitRemote || !strings.Contains(err.Error(), "server digest") {
		t.Errorf("err = %v", err)
	}
}

func TestUploadFailureDeletesDocument(t *testing.T) {
	for _, step := range []string{"upload", "attach"} {
		f := &fakeFiles{uploads: map[string][]byte{}, failStep: step}
		c := filesClient(t, f)
		_, err := c.UploadFile(context.Background(), "/", "a.txt", "a", "", "text/plain", strings.NewReader("abc"))
		if err == nil || !f.deleted {
			t.Errorf("%s fails: err=%v deleted=%v", step, err, f.deleted)
		}
	}
}

func TestDownloadWithoutFile(t *testing.T) {
	f := &fakeFiles{uploads: map[string][]byte{}, name: "folder"}
	_, err := filesClient(t, f).Download(context.Background(), "d1", io.Discard)
	if output.CodeOf(err) != output.ExitUsage {
		t.Errorf("err = %v", err)
	}
}

func TestListChildren(t *testing.T) {
	f := &fakeFiles{uploads: map[string][]byte{}, name: "projects"}
	res, err := filesClient(t, f).ListChildren(context.Background(), "/projects", ListOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if f.lastQuery["query"] != "SELECT * FROM SysContent WHERE sys_parentId = 'd1'" || f.querySync != "true" ||
		f.lastQuery["limit"] != float64(5) || len(res.Documents) != 1 {
		t.Errorf("query = %v sync=%q, documents %d", f.lastQuery, f.querySync, len(res.Documents))
	}
}

func TestReplaceFile(t *testing.T) {
	f := &fakeFiles{uploads: map[string][]byte{}}
	c := filesClient(t, f)
	ptr := func(s string) *string { return &s }
	// No document of that name yet: a new one is uploaded.
	doc, replaced, err := c.ReplaceFile(context.Background(), "/", "a.txt", ptr("a"), ptr("first"), "text/plain", strings.NewReader("one"))
	if err != nil || replaced || doc.ID != "d1" || string(f.blob) != "one" {
		t.Fatalf("first = %+v, %v, %v, blob %q", doc, replaced, err, f.blob)
	}
	// Now it exists: same document, new content and description.
	doc, replaced, err = c.ReplaceFile(context.Background(), "/", "a.txt", ptr("a"), ptr("second"), "text/plain", strings.NewReader("two"))
	if err != nil || !replaced || doc.ID != "d1" || string(f.blob) != "two" || f.description != "second" {
		t.Errorf("second = %+v, %v, %v, blob %q, description %q", doc, replaced, err, f.blob, f.description)
	}
	// Without a description, the content changes and the description stays.
	_, replaced, err = c.ReplaceFile(context.Background(), "/", "a.txt", nil, nil, "text/plain", strings.NewReader("three"))
	if err != nil || !replaced || string(f.blob) != "three" || f.description != "second" {
		t.Errorf("third = %v, %v, blob %q, description %q", replaced, err, f.blob, f.description)
	}
}
