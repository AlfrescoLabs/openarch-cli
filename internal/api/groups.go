package api

import (
	"context"
	"net/http"
)

// GroupExists reports whether OpenArch knows a group id. OpenArch reads its
// groups from the users file only at startup, so a group added to the file
// is unknown until the next 'openarch up'.
func (c *Client) GroupExists(ctx context.Context, id string) (bool, error) {
	what := "get group " + id
	resp, err := c.Gen.GetGroupByIdWithResponse(ctx, id)
	if err != nil {
		return false, c.Transport(err, what)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return false, nil
	}
	if err := c.checkBody(resp.StatusCode(), resp.Body, what); err != nil {
		return false, err
	}
	return true, nil
}
