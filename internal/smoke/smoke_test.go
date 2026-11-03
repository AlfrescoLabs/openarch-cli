package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// fake is an in-memory OpenArch. Fields make single calls fail.
type fake struct {
	pingErr, readErr error
	indexed          bool // the background query finds the document
	docs             map[string]api.Document
	deleted          int
}

func (f *fake) Ping(context.Context) (api.PingResult, error) {
	return api.PingResult{Version: "1.0.90", User: "admin"}, f.pingErr
}

func (f *fake) CreateDocument(_ context.Context, _ string, body []byte) (api.Document, error) {
	var b map[string]string
	_ = json.Unmarshal(body, &b)
	d := api.Document{}
	d.ID, d.Path, d.Name = "id-1", "/"+b["sys_name"], b["sys_name"]
	f.docs = map[string]api.Document{d.ID: d, d.Path: d}
	return d, nil
}

func (f *fake) GetDocument(_ context.Context, ref string) (api.Document, error) {
	if f.readErr != nil {
		return api.Document{}, f.readErr
	}
	d, ok := f.docs[ref]
	if !ok {
		return d, &output.Error{Code: output.ExitRemote, Message: "not found", Err: api.ErrNotFound}
	}
	return d, nil
}

func (f *fake) Query(_ context.Context, o api.QueryOptions) (api.QueryResult, error) {
	if !o.Sync && !f.indexed {
		return api.QueryResult{}, nil
	}
	return api.QueryResult{Documents: []api.Document{f.docs["id-1"]}}, nil
}

func (f *fake) DeleteDocument(_ context.Context, id string) error {
	f.deleted++
	d := f.docs[id]
	delete(f.docs, id)
	delete(f.docs, d.Path)
	return nil
}

func statuses(r Report) map[string]string {
	m := map[string]string{}
	for _, s := range r.Steps {
		m[s.ID] = s.Status
	}
	return m
}

func TestAllPass(t *testing.T) {
	r := Run(context.Background(), &fake{indexed: true}, Options{URL: "http://x", IndexTimeout: time.Second})
	if !r.Passed() || len(r.Steps) != 8 || r.Version != "1.0.90" {
		t.Errorf("report = %+v", r)
	}
}

func TestAuthFailureSkipsTheRest(t *testing.T) {
	f := &fake{pingErr: output.NewError(output.ExitAuth, "authentication failed", "")}
	r := Run(context.Background(), f, Options{IndexTimeout: time.Second})
	if !r.AuthFailed() || r.Passed() || r.Summary.Fail != 1 || r.Summary.Skip != 7 || f.deleted != 0 {
		t.Errorf("report = %+v, deleted = %d", r.Summary, f.deleted)
	}
}

// A failed step after the create still deletes the test document.
func TestCleanupAfterFailure(t *testing.T) {
	f := &fake{indexed: true, readErr: errors.New("boom")}
	r := Run(context.Background(), f, Options{IndexTimeout: time.Second})
	s := statuses(r)
	if s["read-id"] != Fail || s["delete"] != Pass || f.deleted != 1 || r.Passed() {
		t.Errorf("steps = %v, deleted = %d", s, f.deleted)
	}
}

func TestIndexTimeout(t *testing.T) {
	r := Run(context.Background(), &fake{}, Options{IndexTimeout: 30 * time.Millisecond, PollInterval: 10 * time.Millisecond})
	s := statuses(r)
	if s["query"] != Pass || s["index"] != Fail || s["verify-delete"] != Pass || r.Passed() {
		t.Errorf("steps = %v", s)
	}
}
