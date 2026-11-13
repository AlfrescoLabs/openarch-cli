package demo

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/users"
)

func TestAddUser(t *testing.T) {
	f := users.Build("admin", "hash", users.File{})
	u := Users[0]
	if !AddUser(&f, u, "{bcrypt}one") {
		t.Fatal("a new user is a change")
	}
	if !HasGroups(f, u) {
		t.Errorf("groups = %v", f.Users[f.Find(u.Name)].Groups)
	}
	for _, g := range u.ExtraGroups() {
		if !slices.ContainsFunc(f.Groups, func(fg users.Group) bool { return fg.Name == g }) {
			t.Errorf("group %s not created: %v", g, f.Groups)
		}
	}
	if AddUser(&f, u, "{bcrypt}one") {
		t.Error("the same user and hash is not a change")
	}
	if !AddUser(&f, u, "{bcrypt}two") || f.Users[f.Find(u.Name)].Password != "{bcrypt}two" {
		t.Error("a new hash is a change")
	}
	i := f.Find(u.Name)
	f.Users[i].Groups = []string{users.MembersGroup}
	if HasGroups(f, u) || !AddUser(&f, u, "{bcrypt}two") || !HasGroups(f, u) {
		t.Error("a missing demo group is added back")
	}
	if n := len(f.Users); n != 2 {
		t.Errorf("users = %d, want admin and %s", n, u.Name)
	}
}

// fakeClient records the documents that Load creates.
type fakeClient struct {
	known bool
	// unknownUser is a user id that OpenArch does not know.
	unknownUser string
	// stored are the embeddings, by "<document path> <type>".
	stored  map[string][]api.EmbeddingRow
	exists  bool
	deleted []string
	bodies  map[string]map[string]any
}

func (c *fakeClient) GetDocument(_ context.Context, ref string) (api.Document, error) {
	if c.exists && ref == Root {
		return api.Document{}, nil
	}
	return api.Document{}, &output.Error{Code: output.ExitRemote, Message: "not found", Err: api.ErrNotFound}
}

func (c *fakeClient) CreateDocument(_ context.Context, parent string, body []byte) (api.Document, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return api.Document{}, err
	}
	path := strings.TrimSuffix(parent, "/") + "/" + m["sys_name"].(string)
	c.bodies[path] = m
	var d api.Document
	if err := json.Unmarshal([]byte(`{"sys_id":"id-`+path+`","sys_path":"`+path+`"}`), &d); err != nil {
		return api.Document{}, err
	}
	return d, nil
}

func (c *fakeClient) DeleteDocument(_ context.Context, id string) error {
	c.deleted = append(c.deleted, id)
	return nil
}

func (c *fakeClient) GroupExists(context.Context, string) (bool, error) { return c.known, nil }

func (c *fakeClient) UserExists(_ context.Context, id string) (bool, error) {
	return c.known && id != c.unknownUser, nil
}

func (c *fakeClient) StoreEmbeddings(_ context.Context, doc api.Document, typ string, rows []api.EmbeddingRow) error {
	if c.stored == nil {
		c.stored = map[string][]api.EmbeddingRow{}
	}
	c.stored[doc.Path+" "+typ] = rows
	return nil
}

func ids() IDs {
	return IDs{
		Users:  map[string]string{"alice": "alice-id", "bob": "bob-id", "carol": "carol-id"},
		Groups: map[string]string{users.MembersGroup: "m-id", "demo-hr": "hr-id", "demo-finance": "fin-id"},
	}
}

func aclOf(t *testing.T, c *fakeClient, p string) string {
	t.Helper()
	b, err := json.Marshal(c.bodies[p]["sys_acl"])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoad(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	res, err := Load(context.Background(), c, ids(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "loaded" || res.Folders != 6 || res.Documents != 10 || len(c.bodies) != 17 {
		t.Errorf("result = %+v, created %d", res, len(c.bodies))
	}
	for p, want := range map[string]string{
		"/Demo/HR": `[{"granted":true,"group":{"id":"hr-id"},"permission":"Read"}]`,
		// The block comes last: it denies everything that was not granted before it.
		"/Demo/Public/Facilities": `[{"granted":true,"permission":"Read","user":{"id":"bob-id"}},` +
			`{"granted":false,"permission":"Everything","user":{"id":"__Everyone__"}}]`,
		"/Demo/Finance/travel-policy-2027":    `[{"granted":true,"permission":"Read","user":{"id":"alice-id"}}]`,
		"/Demo/Finance/contractor-rates-2027": `[{"end":"2026-01-01T00:00:00Z","granted":true,"permission":"Read","user":{"id":"alice-id"}}]`,
	} {
		if got := aclOf(t, c, p); got != want {
			t.Errorf("%s acl = %s, want %s", p, got, want)
		}
	}
	for _, p := range []string{"/Demo", "/Demo/Board", "/Demo/HR/Reviews", "/Demo/HR/salary-bands-2026"} {
		if _, ok := c.bodies[p]["sys_acl"]; ok {
			t.Errorf("%s must have no access entries of its own", p)
		}
	}
	if c.bodies["/Demo/Finance/budget-forecast-2027"]["sys_primaryType"] != "SysFile" {
		t.Errorf("document = %v", c.bodies["/Demo/Finance/budget-forecast-2027"])
	}
}

func TestLoadExistsAndForce(t *testing.T) {
	c := &fakeClient{known: true, exists: true, bodies: map[string]map[string]any{}}
	res, err := Load(context.Background(), c, ids(), LoadOptions{})
	if err != nil || res.Action != "exists" || len(c.bodies) != 0 {
		t.Errorf("result = %+v, err = %v, created %d", res, err, len(c.bodies))
	}
	res, err = Load(context.Background(), c, ids(), LoadOptions{Force: true})
	if err != nil || res.Action != "loaded" || len(c.deleted) != 1 {
		t.Errorf("result = %+v, err = %v, deleted %v", res, err, c.deleted)
	}
}

func TestLoadNeedsPrincipals(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	missingGroup := ids()
	delete(missingGroup.Groups, "demo-finance")
	if _, err := Load(context.Background(), c, missingGroup, LoadOptions{}); err == nil || !strings.Contains(err.Error(), "group demo-finance is not in") {
		t.Errorf("err = %v", err)
	}
	missingUser := ids()
	delete(missingUser.Users, "bob")
	if _, err := Load(context.Background(), c, missingUser, LoadOptions{}); err == nil || !strings.Contains(err.Error(), "user bob is not in") {
		t.Errorf("err = %v", err)
	}
	var oerr *output.Error
	c.unknownUser = "alice-id"
	if _, err := Load(context.Background(), c, ids(), LoadOptions{}); !errors.As(err, &oerr) || !strings.Contains(oerr.Message, "user alice") || !strings.Contains(oerr.Hint, "openarch up") {
		t.Errorf("err = %v", err)
	}
	c.unknownUser = ""
	c.known = false
	if _, err := Load(context.Background(), c, ids(), LoadOptions{}); !errors.As(err, &oerr) || !strings.Contains(oerr.Hint, "openarch up") {
		t.Errorf("err = %v", err)
	}
	if len(c.bodies) != 0 {
		t.Errorf("created %d documents before the check", len(c.bodies))
	}
}

func TestReset(t *testing.T) {
	c := &fakeClient{}
	if res, err := Reset(context.Background(), c); err != nil || res.Action != "absent" {
		t.Errorf("result = %+v, err = %v", res, err)
	}
	c.exists = true
	if res, err := Reset(context.Background(), c); err != nil || res.Action != "deleted" || len(c.deleted) != 1 {
		t.Errorf("result = %+v, err = %v", res, err)
	}
}

func TestLoadEmbeddings(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	var texts []string
	embed := func(_ context.Context, in []string) ([][]float32, error) {
		texts = in
		out := make([][]float32, len(in))
		for i := range in {
			out[i] = []float32{float32(i)}
		}
		return out, nil
	}
	res, err := Load(context.Background(), c, ids(), LoadOptions{Embed: embed, EmbeddingType: "ai-model"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Embeddings != 10 || res.EmbeddingType != "ai-model" || len(c.stored) != 10 {
		t.Errorf("result = %+v, stored %d", res, len(c.stored))
	}
	rows := c.stored["/Demo/HR/salary-bands-2026 ai-model"]
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Text, "Salary bands 2026. ") || len(texts) != 10 {
		t.Errorf("rows = %+v", rows)
	}
	// OpenArch keeps one vector per row id and type, so the ids must differ between documents.
	seen := map[string]bool{}
	for _, rs := range c.stored {
		if seen[rs[0].ID] {
			t.Errorf("row id %q is used twice", rs[0].ID)
		}
		seen[rs[0].ID] = true
	}
}

func TestLoadEmbeddingsFailFirst(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	embed := func(context.Context, []string) ([][]float32, error) { return nil, errors.New("model not pulled") }
	if _, err := Load(context.Background(), c, ids(), LoadOptions{Embed: embed, EmbeddingType: "t"}); err == nil {
		t.Fatal("no error")
	}
	if len(c.bodies) != 0 {
		t.Errorf("created %d documents before the vectors failed", len(c.bodies))
	}
}
