package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/api/gen"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Blob describes the file of a SysFile document (property sysfile_blob).
// Digest is the SHA-256 of the content, in hex.
type Blob struct {
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Length   int64  `json:"length"`
	Digest   string `json:"digest"`
}

// UploadFile creates a SysFile document under parent and attaches content:
// it creates the document, uploads the content (POST /api/upload), then sets
// sysfile_blob to the upload. When a later step fails, the new document is
// deleted again. An empty description is left out.
func (c *Client) UploadFile(ctx context.Context, parent, name, title, description, mimeType string, content io.Reader) (Document, error) {
	props := map[string]string{"sys_primaryType": "SysFile", "sys_name": name, "sys_title": title}
	if description != "" {
		props["sys_description"] = description
	}
	body, err := json.Marshal(props)
	if err != nil {
		return Document{}, err
	}
	doc, err := c.CreateDocument(ctx, parent, body)
	if err != nil {
		return Document{}, err
	}
	attached, err := c.attach(ctx, doc.ID, name, mimeType, content)
	if err != nil {
		_ = c.DeleteDocument(context.WithoutCancel(ctx), doc.ID)
		return Document{}, err
	}
	return attached, nil
}

func (c *Client) attach(ctx context.Context, id, name, mimeType string, content io.Reader) (Document, error) {
	what := "upload " + name
	resp, err := c.Gen.UploadWithBodyWithResponse(ctx, &gen.UploadParams{FileName: &name, MimeType: &mimeType}, mimeType, content)
	if err != nil {
		return Document{}, c.Transport(err, what)
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, what); err != nil {
		return Document{}, err
	}
	var up struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Body, &up); err != nil || up.ID == "" {
		return Document{}, output.NewError(output.ExitRemote, what+": the server returned no upload id", "")
	}
	// PATCH accepts only JSON Patch, so the blob is set with PUT.
	put, err := json.Marshal(map[string]any{"sysfile_blob": map[string]string{"uploadId": up.ID}})
	if err != nil {
		return Document{}, err
	}
	what = "attach " + name
	presp, err := c.Gen.UpdateDocumentByIdWithBodyWithResponse(ctx, id, &gen.UpdateDocumentByIdParams{}, "application/json", bytes.NewReader(put))
	if err != nil {
		return Document{}, c.Transport(err, what)
	}
	if err := c.checkBody(presp.StatusCode(), presp.Body, what); err != nil {
		return Document{}, err
	}
	return parseDocument(presp.Body)
}

// DownloadResult describes a downloaded file.
type DownloadResult struct {
	Document string `json:"document"`
	Blob
	SHA256 string `json:"sha256"`
}

// Download writes the file of the document ref to w and checks its SHA-256
// against the digest stored by OpenArch.
func (c *Client) Download(ctx context.Context, ref string, w io.Writer) (DownloadResult, error) {
	doc, err := c.GetDocument(ctx, ref)
	if err != nil {
		return DownloadResult{}, err
	}
	if doc.Blob == nil || doc.Blob.Filename == "" {
		return DownloadResult{}, output.Usage(ref+" has no file", "Only documents with a file (SysFile) can be downloaded.")
	}
	what := "download " + ref
	u := c.creds.URL + "/api/download/" + url.PathEscape(doc.ID) + "/sysfile_blob/" + url.PathEscape(doc.Blob.Filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return DownloadResult{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return DownloadResult{}, c.Transport(err, what)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return DownloadResult{}, c.checkBody(resp.StatusCode, body, what)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		return DownloadResult{}, c.Transport(err, what)
	}
	res := DownloadResult{Document: doc.ID, Blob: *doc.Blob, SHA256: hex.EncodeToString(h.Sum(nil))}
	res.Length = n
	if d := strings.ToLower(doc.Blob.Digest); len(d) == 64 && d != res.SHA256 {
		return res, output.NewError(output.ExitRemote,
			fmt.Sprintf("%s: the SHA-256 of the content is %s, but the server digest is %s", what, res.SHA256, d), "Download it again.")
	}
	return res, nil
}

// ListOptions are the inputs of ListChildren.
type ListOptions struct {
	Limit  int64
	Offset int64
}

// ListChildren returns the documents in the folder ref, sorted by name.
// It reads the database (sync), so a new document shows at once.
func (c *Client) ListChildren(ctx context.Context, ref string, o ListOptions) (QueryResult, error) {
	id := ref
	if IsPath(ref) {
		doc, err := c.GetDocument(ctx, ref)
		if err != nil {
			return QueryResult{}, err
		}
		id = doc.ID
	}
	return c.Query(ctx, QueryOptions{
		Query:  "SELECT * FROM SysContent WHERE sys_parentId = '" + strings.ReplaceAll(id, "'", "") + "'",
		Limit:  o.Limit,
		Offset: o.Offset,
		Sort:   []string{"sys_name"},
		Sync:   true,
	})
}
