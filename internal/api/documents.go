package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/api/gen"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// ErrNotFound is wrapped by the errors of calls that got HTTP 404.
var ErrNotFound = errors.New("not found")

// Document is a document as returned by OpenArch. It is kept as raw JSON, so
// that no property is lost, with the fields the CLI shows in text mode decoded.
type Document struct {
	Raw json.RawMessage
	documentSummary
}

type documentSummary struct {
	ID          string `json:"sys_id"`
	PrimaryType string `json:"sys_primaryType"`
	Name        string `json:"sys_name"`
	Title       string `json:"sys_title"`
	Description string `json:"sys_description"`
	Path        string `json:"sys_path"`
	Created     string `json:"sys_created"`
	Modified    string `json:"sys_modified"`
	Creator     *struct {
		Username string `json:"username"`
	} `json:"sys_creator"`
	// Blob is set for documents with a file.
	Blob *Blob `json:"sysfile_blob"`
}

// MarshalJSON writes the document exactly as OpenArch returned it.
func (d Document) MarshalJSON() ([]byte, error) { return d.Raw, nil }

func parseDocument(data []byte) (Document, error) {
	d := Document{Raw: json.RawMessage(bytes.TrimSpace(data))}
	if err := json.Unmarshal(data, &d.documentSummary); err != nil {
		return d, output.NewError(output.ExitRemote, "the server returned a document that is not valid JSON: "+err.Error(), "")
	}
	return d, nil
}

// WriteText writes the human-readable form.
func (d Document) WriteText(w io.Writer) error {
	var b strings.Builder
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%-12s %s\n", k, v)
		}
	}
	row("id", d.ID)
	row("type", d.PrimaryType)
	row("path", d.Path)
	row("name", d.Name)
	row("title", d.Title)
	row("description", d.Description)
	if d.Creator != nil {
		row("creator", d.Creator.Username)
	}
	row("created", d.Created)
	row("modified", d.Modified)
	_, err := io.WriteString(w, b.String())
	return err
}

// IsPath reports whether ref is a document path (it starts with "/") rather than an id.
func IsPath(ref string) bool { return strings.HasPrefix(ref, "/") }

// documentPathURL returns the URL of a document path. "/" is kept as is in
// each segment: the server rejects %2F.
func (c *Client) documentPathURL(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return c.creds.URL + "/api/documents/path/" + strings.Join(segs, "/")
}

// raw sends a request and returns the status and the body.
func (c *Client) raw(ctx context.Context, method, u string, body []byte) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return resp.StatusCode, data, err
}

// checkBody is Check with the server message added, when the body has one.
func (c *Client) checkBody(status int, body []byte, what string) error {
	err := c.Check(status, what)
	if err == nil {
		return nil
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		var oerr *output.Error
		if errors.As(err, &oerr) {
			switch status {
			case http.StatusNotFound:
				oerr.Message = fmt.Sprintf("%s: not found: %s", what, e.Message)
			default:
				oerr.Message += ": " + e.Message
			}
		}
	}
	var oerr *output.Error
	if status == http.StatusNotFound && errors.As(err, &oerr) {
		oerr.Err = ErrNotFound
	}
	return err
}

// DeleteDocument deletes a document by id.
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	what := "delete document " + id
	resp, err := c.Gen.DeleteDocumentByIdWithResponse(ctx, id, &gen.DeleteDocumentByIdParams{})
	if err != nil {
		return c.Transport(err, what)
	}
	return c.checkBody(resp.StatusCode(), resp.Body, what)
}

// GetDocument returns a document by id, or by path when ref starts with "/".
func (c *Client) GetDocument(ctx context.Context, ref string) (Document, error) {
	what := "get document " + ref
	var status int
	var body []byte
	if IsPath(ref) {
		var err error
		status, body, err = c.raw(ctx, http.MethodGet, c.documentPathURL(ref), nil)
		if err != nil {
			return Document{}, c.Transport(err, what)
		}
	} else {
		resp, err := c.Gen.GetDocumentByIdWithResponse(ctx, ref, &gen.GetDocumentByIdParams{})
		if err != nil {
			return Document{}, c.Transport(err, what)
		}
		status, body = resp.StatusCode(), resp.Body
	}
	if err := c.checkBody(status, body, what); err != nil {
		return Document{}, err
	}
	return parseDocument(body)
}

// CreateDocument creates a document under a parent given by id or path. body is a JSON object.
func (c *Client) CreateDocument(ctx context.Context, parent string, body []byte) (Document, error) {
	what := "create document under " + parent
	var status int
	var data []byte
	if IsPath(parent) {
		var err error
		status, data, err = c.raw(ctx, http.MethodPost, c.documentPathURL(parent), body)
		if err != nil {
			return Document{}, c.Transport(err, what)
		}
	} else {
		resp, err := c.Gen.CreateDocumentUnderParentByIdWithBodyWithResponse(ctx, parent, &gen.CreateDocumentUnderParentByIdParams{}, "application/json", bytes.NewReader(body))
		if err != nil {
			return Document{}, c.Transport(err, what)
		}
		status, data = resp.StatusCode(), resp.Body
	}
	if err := c.checkBody(status, data, what); err != nil {
		return Document{}, err
	}
	return parseDocument(data)
}

// UpdateDocument changes the properties in body (a JSON object) of the
// document ref, given by id or path. Properties not in body keep their value.
func (c *Client) UpdateDocument(ctx context.Context, ref string, body []byte) (Document, error) {
	id := ref
	if IsPath(ref) {
		doc, err := c.GetDocument(ctx, ref)
		if err != nil {
			return Document{}, err
		}
		id = doc.ID
	}
	what := "update document " + ref
	resp, err := c.Gen.UpdateDocumentByIdWithBodyWithResponse(ctx, id, &gen.UpdateDocumentByIdParams{}, "application/json", bytes.NewReader(body))
	if err != nil {
		return Document{}, c.Transport(err, what)
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, what); err != nil {
		return Document{}, err
	}
	return parseDocument(resp.Body)
}

// QueryOptions are the inputs of Query.
type QueryOptions struct {
	Query  string
	Syntax string // "" (hxql) or "expr"
	Limit  int64
	Offset int64
	Sort   []string
	// Sync waits until the full-text index has the latest changes.
	Sync bool
	// Total asks the server to count every match.
	Total bool
}

// QueryResult is the result of a query, kept as raw JSON with the documents decoded.
type QueryResult struct {
	Raw        json.RawMessage
	Documents  []Document
	Count      int64
	TotalCount int64
	Offset     int64
	Truncated  bool
}

// MarshalJSON writes the result exactly as OpenArch returned it.
func (r QueryResult) MarshalJSON() ([]byte, error) { return r.Raw, nil }

// Query runs a query (POST /api/query).
func (c *Client) Query(ctx context.Context, o QueryOptions) (QueryResult, error) {
	q := gen.Query{Query: &o.Query}
	if o.Limit > 0 {
		q.Limit = &o.Limit
	}
	if o.Offset > 0 {
		q.Offset = &o.Offset
	}
	if len(o.Sort) > 0 {
		q.Sort = &o.Sort
	}
	if o.Syntax != "" {
		s := gen.QuerySyntax(o.Syntax)
		q.Syntax = &s
	}
	if o.Total {
		q.TrackTotalCount = &o.Total
	}
	params := &gen.GetDocumentsByQueryParams{}
	if o.Sync {
		params.Sync = &o.Sync
	}
	resp, err := c.Gen.GetDocumentsByQueryWithResponse(ctx, params, q)
	if err != nil {
		return QueryResult{}, c.Transport(err, "query")
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, "query"); err != nil {
		return QueryResult{}, err
	}
	var parsed struct {
		Documents             []json.RawMessage `json:"documents"`
		Count                 int64             `json:"count"`
		TotalCount            int64             `json:"totalCount"`
		Offset                int64             `json:"offset"`
		TotalCountIsTruncated bool              `json:"totalCountIsTruncated"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return QueryResult{}, output.NewError(output.ExitRemote, "the server returned a query result that is not valid JSON: "+err.Error(), "")
	}
	res := QueryResult{Raw: json.RawMessage(bytes.TrimSpace(resp.Body)), Count: parsed.Count, TotalCount: parsed.TotalCount,
		Offset: parsed.Offset, Truncated: parsed.TotalCountIsTruncated}
	for _, d := range parsed.Documents {
		doc, err := parseDocument(d)
		if err != nil {
			return QueryResult{}, err
		}
		res.Documents = append(res.Documents, doc)
	}
	return res, nil
}

// WriteText writes one line per document.
func (r QueryResult) WriteText(w io.Writer) error {
	var b strings.Builder
	if len(r.Documents) == 0 {
		b.WriteString("No documents.\n")
	}
	wType := 4
	for _, d := range r.Documents {
		wType = max(wType, len(d.PrimaryType))
	}
	for _, d := range r.Documents {
		label := d.Path
		if label == "" {
			label = d.Name
		}
		if d.Title != "" && d.Title != d.Name {
			label += "  (" + d.Title + ")"
		}
		fmt.Fprintf(&b, "%-36s  %-*s  %s\n", d.ID, wType, d.PrimaryType, label)
	}
	if len(r.Documents) > 0 {
		total := fmt.Sprint(r.TotalCount)
		if r.Truncated {
			total = "more than " + total
		}
		fmt.Fprintf(&b, "\n%d to %d of %s\n", r.Offset+1, r.Offset+int64(len(r.Documents)), total)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
