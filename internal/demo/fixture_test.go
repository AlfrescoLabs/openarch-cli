package demo

import (
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/users"
)

// readPermissions are the permissions that include Read.
var readPermissions = []string{"Read", "ReadWrite", "Everything"}

// effectiveACL returns the entries that decide access to p, in the order
// OpenArch checks them: the entries of p, then those it inherits, unless an
// item on the way blocks inheritance. Nothing after a block gives access.
func effectiveACL(p string, items map[string]item) []ACE {
	var aces []ACE
	for {
		it, ok := items[p]
		if !ok {
			return aces
		}
		aces = append(aces, it.acl...)
		if it.block || p == Root {
			return aces
		}
		p = path.Dir(p)
	}
}

type item struct {
	acl   []ACE
	block bool
}

// canRead applies the rule of OpenArch: the first entry that matches the user
// and gives Read decides, entries that have ended are skipped, and no match
// (or the block) means no access.
func canRead(u User, aces []ACE, now time.Time) bool {
	for _, a := range aces {
		if a.End != "" {
			end, err := time.Parse(time.RFC3339, a.End)
			if err != nil || !end.After(now) {
				continue
			}
		}
		if !slices.Contains(readPermissions, a.Permission) {
			continue
		}
		if a.User == u.Name || (a.Group != "" && slices.Contains(u.Groups, a.Group)) {
			return true
		}
	}
	return false
}

func TestFixtureVisibility(t *testing.T) {
	items := map[string]item{}
	for _, f := range Data.Folders {
		items[f.Path] = item{f.ACL, f.BlockInheritance}
	}
	for _, d := range Data.Documents {
		items[d.Path()] = item{d.ACL, d.BlockInheritance}
	}
	now := time.Now()
	for _, d := range Data.Documents {
		aces := effectiveACL(d.Path(), items)
		var got []string
		for _, u := range Data.Users {
			if canRead(u, aces, now) {
				got = append(got, u.Name)
			}
		}
		want := slices.Clone(d.VisibleTo)
		slices.Sort(want)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: visibleTo %v, the access entries give %v", d.Path(), want, got)
		}
	}
}

func TestFixtureData(t *testing.T) {
	if Data.Version != 1 || Data.Root != Root {
		t.Fatalf("version %d, root %q", Data.Version, Data.Root)
	}
	folders := map[string]bool{}
	for i, f := range Data.Folders {
		// Load creates them in this order, so a parent must come first.
		if (i == 0) != (f.Path == Root) || (i > 0 && !folders[path.Dir(f.Path)]) {
			t.Errorf("folder %s: its parent is not listed before it", f.Path)
		}
		folders[f.Path] = true
	}
	known := map[string]bool{}
	for _, u := range Data.Users {
		if !slices.Contains(u.Groups, users.MembersGroup) {
			t.Errorf("user %s is not in %s", u.Name, users.MembersGroup)
		}
		known["user "+u.Name] = true
		for _, g := range u.Groups {
			known["group "+g] = true
		}
	}
	checkACL := func(where string, aces []ACE) {
		for _, a := range aces {
			key := "group " + a.Group
			if a.User != "" {
				key = "user " + a.User
			}
			if (a.User == "") == (a.Group == "") || !known[key] {
				t.Errorf("%s: entry %+v must name one known user or group", where, a)
			}
			if a.End != "" {
				if _, err := time.Parse(time.RFC3339, a.End); err != nil {
					t.Errorf("%s: end %q: %v", where, a.End, err)
				}
			}
		}
	}
	for _, f := range Data.Folders {
		checkACL(f.Path, f.ACL)
	}
	names, canaries := map[string]bool{}, map[string]bool{}
	for _, d := range Data.Documents {
		checkACL(d.Path(), d.ACL)
		if !folders[d.Folder] {
			t.Errorf("%s: folder is not listed", d.Path())
		}
		if names[d.Name] || canaries[d.Canary] {
			t.Errorf("%s: name or canary is used twice", d.Path())
		}
		names[d.Name], canaries[d.Canary] = true, true
		if d.Canary == "" || !strings.Contains(d.Description, d.Canary) {
			t.Errorf("%s: canary %q is not in the description", d.Path(), d.Canary)
		}
		for _, s := range []string{d.Name, d.Title, d.Description} {
			for _, r := range s {
				if r > 127 {
					t.Errorf("%s: %q is not ASCII", d.Path(), s)
					break
				}
			}
		}
	}
}

// Each shared word must hit documents that different users see, so that a
// query on it shows a leak instead of hiding it behind a poor match.
func TestFixtureSharedWords(t *testing.T) {
	for _, w := range Data.SharedWords {
		zones := map[string]bool{}
		for _, d := range Data.Documents {
			if strings.Contains(strings.ToLower(d.Title+" "+d.Description), strings.ToLower(w)) {
				v := slices.Clone(d.VisibleTo)
				slices.Sort(v)
				zones[strings.Join(v, ",")] = true
			}
		}
		if len(zones) < 2 {
			t.Errorf("shared word %q hits documents of %d visibility set", w, len(zones))
		}
	}
}
