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
	if !strings.HasPrefix(h, "{bcrypt}$2a$") {
		t.Errorf("Hash = %q", h)
	}
	if !Matches(h, "s3cret-pass") || Matches(h, "other") || Matches("{noop}s3cret-pass", "s3cret-pass") {
		t.Error("Matches gave a wrong answer")
	}
}

func TestEnvRoundTrip(t *testing.T) {
	e := Env{AdminUser: "admin", AdminPassword: "abc123"}
	if got := Parse(e.Marshal()); got != e {
		t.Errorf("Parse(Marshal) = %+v", got)
	}
	got := Parse([]byte("# c\nOTHER=1\n OPENARCH_ADMIN_USER = \"root\"\nOPENARCH_ADMIN_PASSWORD='p w'\n"))
	if got.AdminUser != "root" || got.AdminPassword != "p w" {
		t.Errorf("Parse = %+v", got)
	}
}
