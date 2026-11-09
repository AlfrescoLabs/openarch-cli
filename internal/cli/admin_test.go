package cli

import "testing"

func TestSelectsColumns(t *testing.T) {
	cases := []struct {
		syntax, query string
		want          bool
	}{
		{"", "SELECT * FROM SysFile", false},
		{"hxql", "  select *  from SysFile", false},
		{"", "SELECT sys_name FROM SysFile", true},
		{"hxql", "select sys_name, sys_title from SysFile", true},
		{"expr", "SELECT sys_name FROM SysFile", false},
		{"", "SELECT", false},
	}
	for _, c := range cases {
		if got := selectsColumns(c.syntax, c.query); got != c.want {
			t.Errorf("selectsColumns(%q, %q) = %v, want %v", c.syntax, c.query, got, c.want)
		}
	}
}
