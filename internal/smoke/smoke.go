// Package smoke runs an end-to-end check of an OpenArch server: it creates a
// document, reads it, finds it with a query, and deletes it.
package smoke

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// API is the part of the API client that the smoke test uses.
type API interface {
	Ping(ctx context.Context) (api.PingResult, error)
	CreateDocument(ctx context.Context, parent string, body []byte) (api.Document, error)
	GetDocument(ctx context.Context, ref string) (api.Document, error)
	Query(ctx context.Context, o api.QueryOptions) (api.QueryResult, error)
	DeleteDocument(ctx context.Context, id string) error
}

// Step results.
const (
	Pass = "pass"
	Fail = "fail"
	Skip = "skip"
)

// Step is the result of one step.
type Step struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	DurationMs int64  `json:"durationMs"`
}

// Summary counts the steps by result.
type Summary struct {
	Pass int `json:"pass"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

// Report is the result of a run.
type Report struct {
	URL      string  `json:"url"`
	Version  string  `json:"version,omitempty"`
	Document string  `json:"document,omitempty"`
	Steps    []Step  `json:"steps"`
	Summary  Summary `json:"summary"`
	// authFailed tells the CLI to exit with the authentication code.
	authFailed bool
}

// Passed reports whether every step passed.
func (r Report) Passed() bool { return r.Summary.Fail == 0 && r.Summary.Skip == 0 }

// AuthFailed reports whether the run stopped because the credentials were rejected.
func (r Report) AuthFailed() bool { return r.authFailed }

// Options are the inputs of Run.
type Options struct {
	URL string
	// IndexTimeout is how long to wait for the background search index.
	IndexTimeout time.Duration
	// PollInterval is the wait between two index checks.
	PollInterval time.Duration
}

// Run executes the steps. A step that depends on a failed step is skipped.
// The test document is always deleted when it was created.
func Run(ctx context.Context, c API, o Options) Report {
	if o.PollInterval == 0 {
		o.PollInterval = time.Second
	}
	r := &Report{URL: o.URL}
	name := "openarch-smoke-" + time.Now().UTC().Format("20060102-150405") + "-" + randomSuffix()
	path := "/" + name
	var id string

	run := func(stepID, title string, ok bool, f func() (string, error)) bool {
		s := Step{ID: stepID, Title: title}
		if !ok {
			s.Status, s.Message = Skip, "skipped: an earlier step failed"
			r.add(s)
			return false
		}
		start := time.Now()
		msg, err := f()
		s.DurationMs = time.Since(start).Milliseconds()
		if err != nil {
			s.Status, s.Message = Fail, err.Error()
		} else {
			s.Status, s.Message = Pass, msg
		}
		r.add(s)
		return err == nil
	}

	pinged := run("ping", "Connect and authenticate", true, func() (string, error) {
		res, err := c.Ping(ctx)
		if output.CodeOf(err) == output.ExitAuth {
			r.authFailed = true
		}
		if err != nil {
			return "", err
		}
		r.Version = res.Version
		return fmt.Sprintf("OpenArch %s, user %s", res.Version, res.User), nil
	})
	created := run("create", "Create a folder", pinged, func() (string, error) {
		body, _ := json.Marshal(map[string]string{"sys_primaryType": "SysFolder", "sys_name": name, "sys_title": "openarch smoke test"})
		doc, err := c.CreateDocument(ctx, "/", body)
		if err != nil {
			return "", err
		}
		if doc.ID == "" {
			return "", errors.New("the created document has no sys_id")
		}
		id, r.Document = doc.ID, path
		return "created " + path + " (" + id + ")", nil
	})
	run("read-id", "Read it by id", created, func() (string, error) {
		doc, err := c.GetDocument(ctx, id)
		if err != nil {
			return "", err
		}
		if doc.Path != path {
			return "", fmt.Errorf("path is %q, want %q", doc.Path, path)
		}
		return "read " + id, nil
	})
	run("read-path", "Read it by path", created, func() (string, error) {
		doc, err := c.GetDocument(ctx, path)
		if err != nil {
			return "", err
		}
		if doc.ID != id {
			return "", fmt.Errorf("id is %q, want %q", doc.ID, id)
		}
		return "read " + path, nil
	})
	query := fmt.Sprintf("SELECT * FROM SysFolder WHERE sys_name = '%s'", name)
	found := func(res api.QueryResult) bool {
		for _, d := range res.Documents {
			if d.ID == id {
				return true
			}
		}
		return false
	}
	run("query", "Find it with a query (sync)", created, func() (string, error) {
		res, err := c.Query(ctx, api.QueryOptions{Query: query, Sync: true})
		if err != nil {
			return "", err
		}
		if !found(res) {
			return "", errors.New("the query did not return the document")
		}
		return "found with a synchronized query", nil
	})
	run("index", "Find it in the background search index", created, func() (string, error) {
		deadline := time.Now().Add(o.IndexTimeout)
		for {
			res, err := c.Query(ctx, api.QueryOptions{Query: query})
			if err == nil && found(res) {
				return "found by the background indexing", nil
			}
			if time.Now().After(deadline) {
				if err != nil {
					return "", fmt.Errorf("not indexed after %s: %w", o.IndexTimeout, err)
				}
				return "", fmt.Errorf("not indexed after %s", o.IndexTimeout)
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(o.PollInterval):
			}
		}
	})
	// The delete runs whenever the document exists, so a failed run leaves nothing behind.
	deleted := run("delete", "Delete it", created, func() (string, error) {
		if err := c.DeleteDocument(context.WithoutCancel(ctx), id); err != nil {
			return "", err
		}
		return "deleted " + path, nil
	})
	run("verify-delete", "Check that it is gone", deleted, func() (string, error) {
		_, err := c.GetDocument(ctx, id)
		switch {
		case errors.Is(err, api.ErrNotFound):
			return "not found, as expected", nil
		case err != nil:
			return "", err
		}
		return "", errors.New("the document still exists")
	})
	return *r
}

func (r *Report) add(s Step) {
	r.Steps = append(r.Steps, s)
	switch s.Status {
	case Pass:
		r.Summary.Pass++
	case Fail:
		r.Summary.Fail++
	case Skip:
		r.Summary.Skip++
	}
}

func randomSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(b)
}

// WriteText writes the human-readable form.
func (r Report) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Smoke test of %s\n\n", r.URL)
	width := 0
	for _, s := range r.Steps {
		width = max(width, len(s.ID))
	}
	for _, s := range r.Steps {
		fmt.Fprintf(&b, "%-6s %-*s  %s", "["+s.Status+"]", width, s.ID, s.Message)
		if s.Status != Skip {
			fmt.Fprintf(&b, " (%d ms)", s.DurationMs)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n%d pass, %d fail, %d skip\n", r.Summary.Pass, r.Summary.Fail, r.Summary.Skip)
	_, err := io.WriteString(w, b.String())
	return err
}
