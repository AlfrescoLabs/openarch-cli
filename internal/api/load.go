package api

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api/gen"
)

// SetTimeout sets the time limit of each request. Uploads of large files and
// updates that index many passages can take longer than the default minute.
func (c *Client) SetTimeout(d time.Duration) { c.http.Timeout = d }

// Upload sends data to POST /api/upload and returns the upload id. A blob
// property of a document, such as sysfile_blob, then points to it.
func (c *Client) Upload(ctx context.Context, name, mimeType string, data []byte) (string, error) {
	return c.upload(ctx, name, mimeType, "application/octet-stream", bytes.NewReader(data))
}

// CreateChild creates a document under the parent with id parentID. body is a
// JSON object; its sys_name is used as is.
func (c *Client) CreateChild(ctx context.Context, parentID string, body []byte) (Document, error) {
	what := "create document under " + parentID
	enforce := true
	resp, err := c.Gen.CreateDocumentUnderParentByIdWithBodyWithResponse(ctx, parentID,
		&gen.CreateDocumentUnderParentByIdParams{EnforceSysName: &enforce}, "application/json", bytes.NewReader(body))
	if err != nil {
		return Document{}, c.Transport(err, what)
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, what); err != nil {
		return Document{}, err
	}
	return parseDocument(resp.Body)
}

// Model returns the content model of the repository (GET /api/repository/model).
func (c *Client) Model(ctx context.Context) ([]byte, error) {
	resp, err := c.Gen.GetModelWithResponse(ctx, &gen.GetModelParams{})
	if err != nil {
		return nil, c.Transport(err, "get the content model")
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, "get the content model"); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// PatchModel changes the content model with a JSON Patch. With validateOnly
// the server only checks the patch. Only administrators may change the model.
func (c *Client) PatchModel(ctx context.Context, patch []byte, validateOnly bool) error {
	what := "change the content model"
	if validateOnly {
		what = "validate a content model change"
	}
	u := c.creds.URL + "/api/repository/model?validateOnly=" + strconv.FormatBool(validateOnly)
	status, body, err := c.rawType(ctx, http.MethodPatch, u, "application/json-patch+json", patch)
	if err != nil {
		return c.Transport(err, what)
	}
	return c.checkBody(status, body, what)
}
