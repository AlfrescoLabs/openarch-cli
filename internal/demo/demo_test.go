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
	if !slices.ContainsFunc(f.Groups, func(g users.Group) bool { return g.Name == u.Group }) {
		t.Errorf("group %s not created: %v", u.Group, f.Groups)
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
	known   bool
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
	return api.Document{}, nil
}

func (c *fakeClient) DeleteDocument(_ context.Context, id string) error {
	c.deleted = append(c.deleted, id)
	return nil
}

func (c *fakeClient) GroupExists(context.Context, string) (bool, error) { return c.known, nil }

func groups() map[string]string {
	return map[string]string{users.MembersGroup: "m-id", "demo-hr": "hr-id", "demo-finance": "fin-id"}
}

func TestLoad(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	res, err := Load(context.Background(), c, groups(), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "loaded" || res.Folders != 4 || res.Documents != 6 || len(c.bodies) != 11 {
		t.Errorf("result = %+v, created %d", res, len(c.bodies))
	}
	acl, _ := json.Marshal(c.bodies["/Demo/HR"]["sys_acl"])
	if string(acl) != `[{"granted":true,"group":{"id":"hr-id"},"permission":"Read"}]` {
		t.Errorf("HR acl = %s", acl)
	}
	if _, ok := c.bodies["/Demo/Board"]["sys_acl"]; ok {
		t.Error("Board must have no access entries: administrators only")
	}
	if _, ok := c.bodies["/Demo"]["sys_acl"]; ok {
		t.Error("/Demo must have no access entries")
	}
	if c.bodies["/Demo/Finance/budget-forecast-2027"]["sys_primaryType"] != "SysFile" {
		t.Errorf("document = %v", c.bodies["/Demo/Finance/budget-forecast-2027"])
	}
}

func TestLoadExistsAndForce(t *testing.T) {
	c := &fakeClient{known: true, exists: true, bodies: map[string]map[string]any{}}
	res, err := Load(context.Background(), c, groups(), false)
	if err != nil || res.Action != "exists" || len(c.bodies) != 0 {
		t.Errorf("result = %+v, err = %v, created %d", res, err, len(c.bodies))
	}
	res, err = Load(context.Background(), c, groups(), true)
	if err != nil || res.Action != "loaded" || len(c.deleted) != 1 {
		t.Errorf("result = %+v, err = %v, deleted %v", res, err, c.deleted)
	}
}

func TestLoadNeedsGroups(t *testing.T) {
	c := &fakeClient{known: true, bodies: map[string]map[string]any{}}
	g := groups()
	delete(g, "demo-finance")
	if _, err := Load(context.Background(), c, g, false); err == nil || !strings.Contains(err.Error(), "demo-finance is not in") {
		t.Errorf("err = %v", err)
	}
	c.known = false
	_, err := Load(context.Background(), c, groups(), false)
	var oerr *output.Error
	if !errors.As(err, &oerr) || !strings.Contains(oerr.Hint, "openarch up") {
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
