package demo

import (
	"slices"
	"testing"

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
