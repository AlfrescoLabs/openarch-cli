package users

import (
	"regexp"
	"slices"
	"testing"
)

func TestBuildKeepsOtherUsers(t *testing.T) {
	old := File{
		AdministratorsGroups: []string{AdministratorsGroup, "ops"},
		Groups:               []Group{{ID: "x", Name: AdministratorsGroup}, {ID: "g1", Name: "ops"}},
		Users: []User{
			{ID: AdminID, Username: "old-admin-name", Password: "{bcrypt}old", Email: "boss@example.com", Groups: []string{AdministratorsGroup}},
			{ID: "u1", Username: "alice", Password: "{bcrypt}a", Groups: []string{MembersGroup}, Attributes: map[string]string{"dept": "it"}},
		},
	}
	f := Build("admin", "newhash", old)
	if len(f.Users) != 2 || f.Users[0].Username != "admin" || f.Users[0].Password != "{bcrypt}newhash" || f.Users[0].Email != "boss@example.com" {
		t.Errorf("admin = %+v", f.Users[0])
	}
	if f.Users[1].Username != "alice" || f.Users[1].Attributes["dept"] != "it" {
		t.Errorf("alice = %+v", f.Users[1])
	}
	if !slices.Equal(f.AdministratorsGroups, []string{AdministratorsGroup, "ops"}) || len(f.Groups) != 3 {
		t.Errorf("groups = %v, %v", f.AdministratorsGroups, f.Groups)
	}
	// Building twice gives the same file.
	if string(Build("admin", "newhash", f).Marshal()) != string(f.Marshal()) {
		t.Error("Build is not stable")
	}
}

func TestParseRoundTrip(t *testing.T) {
	f := Build("admin", "h", File{})
	got, err := Parse(f.Marshal())
	if err != nil || got.Find("admin") != 0 || !got.IsAdmin(got.Users[0]) || HashOf(got.Users[0].Password) != "h" {
		t.Errorf("Parse = %+v, %v", got, err)
	}
	if HashOf("{noop}password") != "" {
		t.Error("HashOf accepted a {noop} password")
	}
}

func TestNewIDAndNames(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if id := NewID(); !uuid.MatchString(id) || id == NewID() {
		t.Errorf("NewID = %s", id)
	}
	for _, ok := range []string{"alice", "bob.smith", "a@b.c"} {
		if ValidateUsername(ok) != nil {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "a b", "-x", "a/b"} {
		if ValidateUsername(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
