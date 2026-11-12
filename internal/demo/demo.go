// Package demo holds the sample data of 'openarch demo': two users in two
// groups, and folders with different read access, so that one query returns
// different documents for each user.
//
// OpenArch reads its users only at startup, so the users are written to the
// users file before 'openarch up', and the documents are created after it.
package demo

import (
	"slices"

	"github.com/AlfrescoLabs/openarch-cli/internal/users"
)

// User is a demo user and the group that gives it access to its folder.
type User struct {
	Name  string
	Group string
}

// Users are the demo users. Every user is also in the members group.
var Users = []User{
	{Name: "alice", Group: "demo-hr"},
	{Name: "bob", Group: "demo-finance"},
}

// AddUser adds u to f, or updates it, with the password hash and the demo
// groups. It reports whether f changed.
func AddUser(f *users.File, u User, hash string) bool {
	groups := []string{users.MembersGroup, u.Group}
	for _, g := range groups {
		f.EnsureGroup(g)
	}
	i := f.Find(u.Name)
	if i < 0 {
		f.Users = append(f.Users, users.User{ID: users.NewID(), Username: u.Name, Password: hash, Groups: groups})
		return true
	}
	changed := f.Users[i].Password != hash
	f.Users[i].Password = hash
	for _, g := range groups {
		if !slices.Contains(f.Users[i].Groups, g) {
			f.Users[i].Groups = append(f.Users[i].Groups, g)
			changed = true
		}
	}
	return changed
}

// HasGroups reports whether the user named u.Name is in f with the demo groups.
func HasGroups(f users.File, u User) bool {
	i := f.Find(u.Name)
	return i >= 0 && slices.Contains(f.Users[i].Groups, users.MembersGroup) && slices.Contains(f.Users[i].Groups, u.Group)
}
