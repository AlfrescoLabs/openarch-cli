package localenv

import (
	"strings"
	"testing"
)

func TestPassword(t *testing.T) {
	a, b := Password(24), Password(24)
	if len(a) != 24 || a == b {
		t.Fatalf("Password = %q, %q", a, b)
	}
	for _, r := range a {
		if !strings.ContainsRune(alphabet, r) {
			t.Errorf("unexpected character %q", r)
		}
	}
}

func TestHashMatches(t *testing.T) {
	h, err := Hash("s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$2a$") {
		t.Errorf("Hash = %q", h)
	}
	if !Matches(h, "s3cret-pass") || Matches(h, "other") || Matches("s3cret-pass", "s3cret-pass") {
		t.Error("Matches gave a wrong answer")
	}
}

func TestFill(t *testing.T) {
	e := Env{AdminPassword: "kept"}
	set := e.Fill("admin")
	if e.AdminUser != "admin" || e.AdminPassword != "kept" || len(e.MongoRootPassword) != PasswordLength || e.MongoRootPassword == e.MongoAppPassword {
		t.Errorf("Fill = %+v", e)
	}
	if len(set) != 6 {
		t.Errorf("Fill set %v", set)
	}
	if again := e.Fill("admin"); len(again) != 0 {
		t.Errorf("second Fill set %v", again)
	}
	if !strings.Contains(string(e.Marshal()), MongoAppURIKey+"=mongodb://openarch:"+e.MongoAppPassword+"@mongo:27017/?authSource=openarch\n") {
		t.Errorf("Marshal has no app URI:\n%s", e.Marshal())
	}
}

func TestEnvRoundTrip(t *testing.T) {
	e := Env{AdminUser: "admin", AdminPassword: "abc123"}
	e.Fill("admin")
	if got := Parse(e.Marshal()); got != e {
		t.Errorf("Parse(Marshal) = %+v", got)
	}
	got := Parse([]byte("# c\nOTHER=1\n OPENARCH_ADMIN_USER = \"root\"\nOPENARCH_ADMIN_PASSWORD='p w'\n"))
	if got.AdminUser != "root" || got.AdminPassword != "p w" {
		t.Errorf("Parse = %+v", got)
	}
}
