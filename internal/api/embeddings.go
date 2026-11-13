package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/AlfrescoLabs/openarch-cli/internal/api/gen"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// VectorDimension is the size of the vector index of OpenArch. It pads shorter
// vectors when it stores them, but not in a query.
const VectorDimension = 1024

// embeddingsMixin is the mixin a document needs before it can have embeddings.
const embeddingsMixin = "SysHasEmbeddings"

// EmbeddingType returns the embedding type for a model, named as App for
// OpenArch names it: lowercase, and every character other than a-z, 0-9, '.',
// '_' and '-' becomes '-'. For example ai/mxbai-embed-large gives
// ai-mxbai-embed-large. A query matches the vectors of one type.
func EmbeddingType(model string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(model) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		if !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// EmbeddingRow is one vector of a document, with the text it was made from.
// The ID must be unique among all the vectors of its type, in every document:
// OpenArch turns an ID that is not a UUID into a UUID made from the type and
// the ID, and a vector with the ID of another one replaces it.
type EmbeddingRow struct {
	ID     string
	Text   string
	Vector []float32
}

// parquetRow is one row of the file OpenArch reads. It reads every column by
// name, so all of them must be in the file, also the ones that are empty: a
// file without one is read as no embeddings at all, with no error.
type parquetRow struct {
	ID       string    `parquet:"id"`
	Type     *string   `parquet:"type,optional"`
	Vector   []float64 `parquet:"vector,list"`
	Text     *string   `parquet:"text,optional"`
	Location *string   `parquet:"location,optional"`
	Blobref  *string   `parquet:"blobref,optional"`
}

// EncodeEmbeddings writes rows as the Parquet file that OpenArch reads for a
// SysEmbeddings document.
func EncodeEmbeddings(typ string, rows []EmbeddingRow) ([]byte, error) {
	out := make([]parquetRow, len(rows))
	for i, r := range rows {
		if len(r.Vector) > VectorDimension {
			return nil, fmt.Errorf("vector %s has %d dimensions, OpenArch takes at most %d", r.ID, len(r.Vector), VectorDimension)
		}
		v := make([]float64, len(r.Vector))
		for j, x := range r.Vector {
			v[j] = float64(x)
		}
		out[i] = parquetRow{ID: r.ID, Type: &typ, Vector: v, Text: &r.Text}
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[parquetRow](&buf, parquet.Compression(&parquet.Snappy))
	if _, err := w.Write(out); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StoreEmbeddings stores rows as the embeddings of type typ of doc, the way
// App for OpenArch does: doc gets the SysHasEmbeddings mixin, the Parquet file
// is uploaded, and the child _e_<typ> points to it. An existing child gets the
// new file, so the call can run again.
func (c *Client) StoreEmbeddings(ctx context.Context, doc Document, typ string, rows []EmbeddingRow) error {
	data, err := EncodeEmbeddings(typ, rows)
	if err != nil {
		return err
	}
	if !slices.Contains(doc.Mixins, embeddingsMixin) {
		body, err := json.Marshal(map[string]any{"sys_mixinTypes": append(slices.Clone(doc.Mixins), embeddingsMixin)})
		if err != nil {
			return err
		}
		if _, err := c.UpdateDocument(ctx, doc.ID, body); err != nil {
			return err
		}
	}
	id, err := c.upload(ctx, "embeddings.parquet", "application/x-parquet", "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return err
	}
	blob := map[string]any{"sysemb_embeddings": map[string]string{"uploadId": id}}
	name := "_e_" + typ
	child, err := c.GetDocument(ctx, strings.TrimSuffix(doc.Path, "/")+"/"+name)
	switch {
	case err == nil:
		body, err := json.Marshal(blob)
		if err != nil {
			return err
		}
		_, err = c.UpdateDocument(ctx, child.ID, body)
		return err
	case !errors.Is(err, ErrNotFound):
		return err
	}
	blob["sys_primaryType"] = "SysEmbeddings"
	blob["sys_name"] = name
	blob["sys_title"] = "Embeddings"
	body, err := json.Marshal(blob)
	if err != nil {
		return err
	}
	what := "create " + name + " under " + doc.Path
	enforce := true
	resp, err := c.Gen.CreateDocumentUnderParentByIdWithBodyWithResponse(ctx, doc.ID,
		&gen.CreateDocumentUnderParentByIdParams{EnforceSysName: &enforce}, "application/json", bytes.NewReader(body))
	if err != nil {
		return c.Transport(err, what)
	}
	return c.checkBody(resp.StatusCode(), resp.Body, what)
}

// EmbeddingHit is one vector that a search found.
type EmbeddingHit struct {
	// DocID is the id of the document the vector belongs to.
	DocID string  `json:"docId"`
	Score float64 `json:"score"`
	Text  string  `json:"text,omitempty"`
}

// SearchEmbeddings returns the vectors of type typ nearest to vector, among the
// documents that query (HXQL) selects and the caller may read. An empty query
// searches every document.
func (c *Client) SearchEmbeddings(ctx context.Context, vector []float32, typ, query string, limit int64) ([]EmbeddingHit, int64, error) {
	if len(vector) > VectorDimension {
		return nil, 0, output.NewError(output.ExitUsage, fmt.Sprintf("the query vector has %d dimensions, OpenArch takes at most %d", len(vector), VectorDimension), "")
	}
	v := make([]float64, VectorDimension)
	for i, x := range vector {
		v[i] = float64(x)
	}
	track := true
	q := gen.VectorQuery{Vector: v, EmbeddingType: &typ, Limit: &limit, TrackTotalCount: &track}
	if query != "" {
		q.Query = &query
	}
	resp, err := c.Gen.GetEmbeddingsByQueryWithResponse(ctx, q)
	if err != nil {
		return nil, 0, c.Transport(err, "vector search")
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, "vector search"); err != nil {
		return nil, 0, err
	}
	var r struct {
		Embeddings []struct {
			DocID string  `json:"sysembed_docId"`
			Score float64 `json:"sysembed_score"`
			Text  string  `json:"sysembed_text"`
		} `json:"embeddings"`
		TotalCount int64 `json:"totalCount"`
	}
	if err := json.Unmarshal(resp.Body, &r); err != nil {
		return nil, 0, fmt.Errorf("vector search: %w", err)
	}
	hits := make([]EmbeddingHit, len(r.Embeddings))
	for i, e := range r.Embeddings {
		hits[i] = EmbeddingHit{DocID: e.DocID, Score: e.Score, Text: e.Text}
	}
	return hits, r.TotalCount, nil
}
