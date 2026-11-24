// Package loader loads a corpus into OpenArch from load plans: JSON files that
// list the documents to store, their metadata and their passages. The loader
// makes the passage vectors with a local model and stores them as the
// embeddings of each document. A load can run again: what is already stored is
// left as it is.
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
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Plan is one load plan: usually one source, such as a session record, with
// its original file and the documents made from it.
type Plan struct {
	// Original is the source file as published, stored as is. Optional.
	Original *Original `json:"original"`
	// Documents are the passage documents.
	Documents []Document `json:"documents"`
}

// Original is a file stored as a SysFile document at Path.
type Original struct {
	Path string `json:"path"`
	// File is the local file, relative to the root of the load.
	File     string         `json:"file"`
	Metadata map[string]any `json:"metadata"`
	// MimeType is optional; the default comes from the file name or the content.
	MimeType string `json:"mimeType"`
}

// Document is a SysFile document at Path whose file is the text of its
// passages, one after the other with a blank line between them.
type Document struct {
	Path     string           `json:"path"`
	Metadata map[string]any   `json:"metadata"`
	Passages []map[string]any `json:"passages"`
}

// A passage has an "id", unique in the whole repository, a "text" and a
// "location", for example {"text":{"page":13,"paragraph":12}}. Other keys are
// kept: they are part of the fingerprint of the embeddings.

// ReadPlan reads a plan file. Numbers keep their exact form.
func ReadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return Plan{}, fmt.Errorf("%s: %w", path, err)
	}
	if p.Original != nil && (p.Original.Path == "" || p.Original.File == "") {
		return Plan{}, fmt.Errorf("%s: the original needs a path and a file", path)
	}
	for i, d := range p.Documents {
		if !strings.HasPrefix(d.Path, "/") {
			return Plan{}, fmt.Errorf("%s: document %d has no absolute path", path, i+1)
		}
		for j, ps := range d.Passages {
			id, _ := ps["id"].(string)
			text, _ := ps["text"].(string)
			if id == "" || text == "" {
				return Plan{}, fmt.Errorf("%s: %s: passage %d needs an id and a text", path, d.Path, j+1)
			}
		}
	}
	return p, nil
}

// Client is the part of the API client that the loader uses.
type Client interface {
	GetDocument(ctx context.Context, ref string) (api.Document, error)
	CreateChild(ctx context.Context, parentID string, body []byte) (api.Document, error)
	UpdateDocument(ctx context.Context, ref string, body []byte) (api.Document, error)
	Upload(ctx context.Context, name, mimeType string, data []byte) (string, error)
	Model(ctx context.Context) ([]byte, error)
	PatchModel(ctx context.Context, patch []byte, validateOnly bool) error
}

// Embedder makes one vector per text.
type Embedder func(ctx context.Context, texts []string) ([][]float32, error)

// Options are the options of Load.
type Options struct {
	// Model is the embedding model; it is part of the fingerprint.
	Model         string
	EmbeddingType string
	Embed         Embedder
	// Batch is the number of passages per embedding request.
	Batch int
	// MaxPassageBytes refuses longer passages (UTF-8 bytes) instead of letting
	// the model cut them.
	MaxPassageBytes int
	// Mixins are added to every document.
	Mixins []string
	// HashField is a metadata field with the SHA-256 of the original file.
	// When set, the original file must have this hash, and a stored document
	// with another value is never overwritten.
	HashField string
	// Schema, when set, is a content model fragment with "schemas" and
	// "mixinTypes". What is missing is added; nothing is changed.
	Schema []byte
	// Root is the folder that original file names are relative to.
	Root string
	// Cache is a folder of Parquet files by fingerprint. Empty: no cache.
	Cache string
	// Done is a folder with one marker per loaded plan. Plans with a marker
	// are skipped unless All is set. Empty: no markers.
	Done    string
	All     bool
	Workers int
	// Progress gets one line per finished plan.
	Progress io.Writer
}

// Counts are the counts of a load.
type Counts struct {
	Documents int `json:"documents"`
	Passages  int `json:"passages"`
	// Created, Uploaded (new file), MetadataOnly and Unchanged count documents,
	// originals included.
	Created      int `json:"created"`
	Uploaded     int `json:"uploaded"`
	MetadataOnly int `json:"metadataOnly"`
	Unchanged    int `json:"unchanged"`
	// EmbeddingsStored counts documents whose embeddings were stored;
	// EmbeddingsUnchanged documents whose stored embeddings already match.
	EmbeddingsStored    int `json:"embeddingsStored"`
	EmbeddingsUnchanged int `json:"embeddingsUnchanged"`
	// Embedded counts passages sent to the model; the others came from the cache.
	Embedded int `json:"embeddedPassages"`
}

func (c *Counts) add(o Counts) {
	c.Documents += o.Documents
	c.Passages += o.Passages
	c.Created += o.Created
	c.Uploaded += o.Uploaded
	c.MetadataOnly += o.MetadataOnly
	c.Unchanged += o.Unchanged
	c.EmbeddingsStored += o.EmbeddingsStored
	c.EmbeddingsUnchanged += o.EmbeddingsUnchanged
	c.Embedded += o.Embedded
}

// Result is the result of Load.
type Result struct {
	Plans int `json:"plans"`
	// Loaded plans; Skipped plans already had a marker.
	Loaded  int `json:"loaded"`
	Skipped int `json:"skipped"`
	Counts
	// ModelOperations is the number of content model changes applied.
	ModelOperations int     `json:"modelOperations"`
	ElapsedSeconds  float64 `json:"elapsedSeconds"`
	// Errors are by plan file; other plans are still loaded.
	Errors map[string]string `json:"errors"`
}

func (r Result) WriteText(w io.Writer) error {
	msg := fmt.Sprintf("Loaded %d of %d plans (%d skipped, %d failed): %d documents, %d passages in %.0f s.\n"+
		"Documents: %d created, %d with a new file, %d metadata only, %d unchanged.\n"+
		"Embeddings: %d stored, %d unchanged; %d passages embedded, the rest from the cache.\n",
		r.Loaded, r.Plans, r.Skipped, len(r.Errors), r.Documents, r.Passages, r.ElapsedSeconds,
		r.Created, r.Uploaded, r.MetadataOnly, r.Unchanged, r.EmbeddingsStored, r.EmbeddingsUnchanged, r.Embedded)
	if r.ModelOperations > 0 {
		msg += fmt.Sprintf("Content model: %d additions.\n", r.ModelOperations)
	}
	names := make([]string, 0, len(r.Errors))
	for n := range r.Errors {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		msg += fmt.Sprintf("Failed %s: %s\n", n, r.Errors[n])
	}
	_, err := io.WriteString(w, msg)
	return err
}

// Load loads the plan files. It first embeds the longest passage, so a model
// that cannot take it fails before the repository changes, then adds what the
// content model misses, then loads the plans in parallel.
func Load(ctx context.Context, c Client, files []string, o Options) (Result, error) {
	start := time.Now()
	res := Result{Plans: len(files), Errors: map[string]string{}}
	var todo []string
	for _, f := range files {
		if o.Done != "" && !o.All {
			if _, err := os.Stat(filepath.Join(o.Done, filepath.Base(f))); err == nil {
				res.Skipped++
				continue
			}
		}
		todo = append(todo, f)
	}
	if len(todo) == 0 {
		return res, nil
	}
	if err := preflight(ctx, todo, o); err != nil {
		return res, err
	}
	if o.Schema != nil {
		n, err := EnsureModel(ctx, c, o.Schema)
		if err != nil {
			return res, err
		}
		res.ModelOperations = n
	}
	l := &loader{c: c, o: o, folders: map[string]string{}}
	var mu sync.Mutex
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range max(o.Workers, 1) {
		wg.Go(func() {
			for f := range jobs {
				counts, err := l.plan(ctx, f)
				if err == nil && o.Done != "" {
					err = writeMarker(filepath.Join(o.Done, filepath.Base(f)), counts)
				}
				mu.Lock()
				if err != nil {
					res.Errors[filepath.Base(f)] = err.Error()
				} else {
					res.Loaded++
					res.add(counts)
				}
				if o.Progress != nil {
					s := time.Since(start).Seconds()
					_, _ = fmt.Fprintf(o.Progress, "Loaded %d/%d plans, %d documents, %d passages, %.0f s, %.1f passages/s, %d errors\n",
						res.Loaded, len(todo), res.Documents, res.Passages, s, float64(res.Passages)/max(s, 1), len(res.Errors))
				}
				mu.Unlock()
			}
		})
	}
	for _, f := range todo {
		if ctx.Err() != nil {
			break
		}
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	res.ElapsedSeconds = time.Since(start).Seconds()
	return res, ctx.Err()
}

// preflight embeds the longest passage of the plans (by UTF-8 bytes).
func preflight(ctx context.Context, files []string, o Options) error {
	longest := ""
	for _, f := range files {
		p, err := ReadPlan(f)
		if err != nil {
			return output.NewError(output.ExitUsage, err.Error(), "")
		}
		for _, d := range p.Documents {
			for _, ps := range d.Passages {
				if t := ps["text"].(string); len(t) > len(longest) {
					longest = t
				}
			}
		}
	}
	if longest == "" {
		return nil
	}
	if _, err := embed(ctx, o, []string{longest}); err != nil {
		return fmt.Errorf("embedding check with the longest passage (%d bytes): %w", len(longest), err)
	}
	return nil
}

func writeMarker(path string, c Counts) error {
	data, err := json.MarshalIndent(map[string]any{"documents": c.Documents, "passages": c.Passages,
		"at": time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

type loader struct {
	c Client
	o Options
	// folders are the folders known to exist, path to id. Creating a folder
	// is not safe to race, so mu covers the lookup and the creation.
	mu      sync.Mutex
	folders map[string]string
}

func (l *loader) plan(ctx context.Context, file string) (Counts, error) {
	var n Counts
	p, err := ReadPlan(file)
	if err != nil {
		return n, err
	}
	if o := p.Original; o != nil {
		data, err := os.ReadFile(filepath.Join(l.o.Root, o.File))
		if err != nil {
			return n, err
		}
		if l.o.HashField != "" {
			if want, _ := o.Metadata[l.o.HashField].(string); sha(data) != want {
				return n, fmt.Errorf("%s changed after the plan was made: its SHA-256 is not %s", o.File, l.o.HashField)
			}
		}
		mt := o.MimeType
		if mt == "" {
			mt = mimeType(o.File, data)
		}
		if _, err := l.upsert(ctx, &n, o.Path, o.Metadata, data, mt, nil); err != nil {
			return n, err
		}
	}
	for _, d := range p.Documents {
		if err := l.document(ctx, &n, d); err != nil {
			return n, fmt.Errorf("%s: %w", d.Path, err)
		}
	}
	return n, nil
}

func mimeType(name string, data []byte) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	t, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	return t
}

// document stores a passage document and its embeddings.
func (l *loader) document(ctx context.Context, n *Counts, d Document) error {
	texts := make([]string, len(d.Passages))
	for i, p := range d.Passages {
		texts[i] = p["text"].(string)
	}
	text := strings.Join(texts, "\n\n")
	doc, err := l.upsert(ctx, n, d.Path, d.Metadata, []byte(text), "text/plain", &text)
	if err != nil {
		return err
	}
	n.Documents++
	n.Passages += len(d.Passages)
	fp, err := Fingerprint(l.o.Model, d.Passages)
	if err != nil {
		return err
	}
	name := "_e_" + l.o.EmbeddingType
	child, err := l.get(ctx, d.Path+"/"+name)
	if err != nil {
		return err
	}
	if child != nil && child["sys_description"] == fp && child["sysemb_embeddings"] != nil {
		n.EmbeddingsUnchanged++
		return nil
	}
	data, embedded, err := l.parquet(ctx, d, texts, fp)
	if err != nil {
		return err
	}
	n.Embedded += embedded
	id, err := l.c.Upload(ctx, "embeddings.parquet", "application/x-parquet", data)
	if err != nil {
		return err
	}
	props := map[string]any{"sysemb_embeddings": map[string]string{"uploadId": id}, "sys_description": fp}
	if child != nil {
		_, err = l.c.UpdateDocument(ctx, child["sys_id"].(string), mustJSON(props))
	} else {
		props["sys_primaryType"] = "SysEmbeddings"
		props["sys_name"] = name
		props["sys_title"] = "Embeddings"
		_, err = l.c.CreateChild(ctx, doc, mustJSON(props))
	}
	if err == nil {
		n.EmbeddingsStored++
	}
	return err
}

// parquet returns the embeddings file of d from the cache, or makes it.
func (l *loader) parquet(ctx context.Context, d Document, texts []string, fp string) ([]byte, int, error) {
	cache := ""
	if l.o.Cache != "" {
		cache = filepath.Join(l.o.Cache, fp+".parquet")
		if data, err := os.ReadFile(cache); err == nil {
			return data, 0, nil
		}
	}
	rows := make([]api.EmbeddingRow, len(d.Passages))
	batch := max(l.o.Batch, 1)
	for off := 0; off < len(texts); off += batch {
		end := min(off+batch, len(texts))
		v, err := embed(ctx, l.o, texts[off:end])
		if err != nil {
			return nil, 0, fmt.Errorf("passage batch starting at %d: %w", off+1, err)
		}
		for i := range v {
			p := d.Passages[off+i]
			rows[off+i] = api.EmbeddingRow{ID: p["id"].(string), Text: texts[off+i], Vector: v[i]}
			if loc, ok := p["location"]; ok {
				var b bytes.Buffer
				if err := writePython(&b, loc); err != nil {
					return nil, 0, err
				}
				rows[off+i].Location = b.String()
			}
		}
	}
	data, err := api.EncodeEmbeddings(l.o.EmbeddingType, rows)
	if err != nil {
		return nil, 0, err
	}
	if cache != "" {
		if err := writeAtomic(cache, data); err != nil {
			return nil, 0, err
		}
	}
	return data, len(texts), nil
}

// embed checks the passage length and the vectors the model returns.
func embed(ctx context.Context, o Options, texts []string) ([][]float32, error) {
	for _, t := range texts {
		if o.MaxPassageBytes > 0 && len(t) > o.MaxPassageBytes {
			return nil, fmt.Errorf("a passage has %d bytes, more than %d: split it in the plan, the model would cut it", len(t), o.MaxPassageBytes)
		}
	}
	v, err := o.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(v) != len(texts) {
		return nil, fmt.Errorf("the model returned %d vectors for %d passages", len(v), len(texts))
	}
	for _, x := range v {
		if len(x) == 0 || len(x) > api.VectorDimension {
			return nil, fmt.Errorf("the model returned a vector of %d dimensions; OpenArch takes 1 to %d", len(x), api.VectorDimension)
		}
		for _, f := range x {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return nil, errors.New("the model returned a vector with a value that is not finite")
			}
		}
	}
	return v, nil
}

// upsert stores a SysFile document at path with the metadata and the file
// data, and returns its id. A stored document that already has the metadata
// and a file is left as it is; one with the same file gets the metadata only.
// fulltext, when set, is sent as sys_fulltextBinary and the document gets the
// SysHasEmbeddings mixin.
func (l *loader) upsert(ctx context.Context, n *Counts, path string, metadata map[string]any, data []byte, mimeType string, fulltext *string) (string, error) {
	doc, err := l.get(ctx, path)
	if err != nil {
		return "", err
	}
	if doc != nil && l.o.HashField != "" && !equal(doc[l.o.HashField], metadata[l.o.HashField]) {
		return "", fmt.Errorf("the stored document has another %s: the source changed; it is not overwritten", l.o.HashField)
	}
	name := path[strings.LastIndex(path, "/")+1:]
	props := map[string]any{}
	for k, v := range metadata {
		props[k] = v
	}
	mixins := slices.Clone(l.o.Mixins)
	if fulltext != nil {
		props["sys_fulltextBinary"] = *fulltext
		mixins = append(mixins, "SysHasEmbeddings")
	}
	// The title is the last path segment, unless the plan gives one.
	if _, ok := props["sys_title"]; !ok {
		props["sys_title"] = name
	}
	blob, _ := doc["sysfile_blob"].(map[string]any)
	if doc != nil {
		if unchanged(doc, props, mixins) && blob != nil {
			n.Unchanged++
			return doc["sys_id"].(string), nil
		}
		// Keep the mixins OpenArch added itself.
		for _, m := range stringList(doc["sys_mixinTypes"]) {
			if !slices.Contains(mixins, m) {
				mixins = append(mixins, m)
			}
		}
	}
	if len(mixins) > 0 {
		props["sys_mixinTypes"] = mixins
	}
	if doc != nil && blob != nil && blob["digest"] == sha(data) {
		n.MetadataOnly++
		_, err := l.c.UpdateDocument(ctx, doc["sys_id"].(string), mustJSON(props))
		return doc["sys_id"].(string), err
	}
	id, err := l.c.Upload(ctx, name, mimeType, data)
	if err != nil {
		return "", err
	}
	props["sysfile_blob"] = map[string]string{"uploadId": id}
	n.Uploaded++
	if doc != nil {
		_, err := l.c.UpdateDocument(ctx, doc["sys_id"].(string), mustJSON(props))
		return doc["sys_id"].(string), err
	}
	parent, err := l.folder(ctx, path[:strings.LastIndex(path, "/")])
	if err != nil {
		return "", err
	}
	props["sys_name"] = name
	props["sys_primaryType"] = "SysFile"
	created, err := l.c.CreateChild(ctx, parent, mustJSON(props))
	if err != nil {
		return "", err
	}
	n.Created++
	return created.ID, nil
}

// folder returns the id of the folder at path, and creates what is missing.
func (l *loader) folder(ctx context.Context, path string) (string, error) {
	if path == "" {
		path = "/"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.folderLocked(ctx, path)
}

func (l *loader) folderLocked(ctx context.Context, path string) (string, error) {
	if id, ok := l.folders[path]; ok {
		return id, nil
	}
	doc, err := l.get(ctx, path)
	if err != nil {
		return "", err
	}
	if doc != nil {
		l.folders[path] = doc["sys_id"].(string)
		return l.folders[path], nil
	}
	parentPath, name := path[:strings.LastIndex(path, "/")], path[strings.LastIndex(path, "/")+1:]
	if parentPath == "" {
		parentPath = "/"
	}
	parent, err := l.folderLocked(ctx, parentPath)
	if err != nil {
		return "", err
	}
	created, err := l.c.CreateChild(ctx, parent, mustJSON(map[string]string{
		"sys_primaryType": "SysFolder", "sys_name": name, "sys_title": name}))
	if err != nil {
		return "", err
	}
	l.folders[path] = created.ID
	return created.ID, nil
}

// get returns the document at path as a map with exact numbers, or nil when
// there is none.
func (l *loader) get(ctx context.Context, path string) (map[string]any, error) {
	doc, err := l.c.GetDocument(ctx, path)
	if errors.Is(err, api.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(doc.Raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	if id, _ := m["sys_id"].(string); id == "" {
		return nil, fmt.Errorf("get %s: the server returned a document without sys_id", path)
	}
	return m, nil
}

// unchanged reports whether the stored document already has the properties.
// OpenArch never returns sys_fulltextBinary, returns dates in another form and
// adds mixins of its own.
func unchanged(doc, props map[string]any, mixins []string) bool {
	stored := stringList(doc["sys_mixinTypes"])
	for _, m := range mixins {
		if !slices.Contains(stored, m) {
			return false
		}
	}
	for k, v := range props {
		if k != "sys_fulltextBinary" && !equal(doc[k], v) {
			return false
		}
	}
	return true
}

// equal compares two JSON values: numbers by value and timestamps by time.
func equal(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		fx, err1 := strconv.ParseFloat(string(x), 64)
		fy, err2 := strconv.ParseFloat(string(y), 64)
		return err1 == nil && err2 == nil && fx == fy
	case string:
		y, ok := b.(string)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		tx, err1 := time.Parse(time.RFC3339Nano, x)
		ty, err2 := time.Parse(time.RFC3339Nano, y)
		return err1 == nil && err2 == nil && tx.Equal(ty)
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func stringList(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sha(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // only maps of JSON values reach here
	}
	return data
}
