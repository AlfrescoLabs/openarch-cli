// Package demo holds the sample data of 'openarch demo': two users in two
// groups, and folders with different read access, so that one query returns
// different documents for each user.
//
// OpenArch reads its users only at startup, so the users are written to the
// users file before 'openarch up', and the documents are created after it.
package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
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

// Root is the folder that holds the demo documents.
const Root = "/Demo"

// Doc is a demo document. It has metadata only: OpenArch searches metadata,
// and does not extract text from files.
type Doc struct {
	Name        string
	Title       string
	Description string
}

// Folder is a demo folder. Group is the group that may read it, and "" means
// administrators only: the root folder has no access entries, so only
// administrators read what has none.
type Folder struct {
	Name  string
	Group string
	Docs  []Doc
}

// Folders are the demo folders under Root.
var Folders = []Folder{
	{Name: "Public", Group: users.MembersGroup, Docs: []Doc{
		{Name: "holiday-calendar-2026", Title: "Holiday calendar 2026", Description: "Public holidays and office closures in 2026"},
		{Name: "office-wifi-guide", Title: "Office Wi-Fi guide", Description: "How to connect to the office Wi-Fi"},
	}},
	{Name: "HR", Group: "demo-hr", Docs: []Doc{
		{Name: "salary-bands-2026", Title: "Salary bands 2026", Description: "Salary ranges per job level"},
		{Name: "hiring-plan-q1-2027", Title: "Hiring plan Q1 2027", Description: "Open positions and hiring budget"},
	}},
	{Name: "Finance", Group: "demo-finance", Docs: []Doc{
		{Name: "budget-forecast-2027", Title: "Budget forecast 2027", Description: "Revenue and cost forecast"},
	}},
	{Name: "Board", Docs: []Doc{
		{Name: "merger-proposal", Title: "Merger proposal", Description: "Confidential board proposal"},
	}},
}

// Client is the part of the API client that Load and Reset use.
type Client interface {
	GetDocument(ctx context.Context, ref string) (api.Document, error)
	CreateDocument(ctx context.Context, parent string, body []byte) (api.Document, error)
	DeleteDocument(ctx context.Context, id string) error
	GroupExists(ctx context.Context, id string) (bool, error)
}

// Result is the result of Load and Reset.
type Result struct {
	// Action is "loaded", "exists", "deleted" or "absent".
	Action    string   `json:"action"`
	Root      string   `json:"root"`
	Folders   int      `json:"folders"`
	Documents int      `json:"documents"`
	Created   []string `json:"created,omitempty"`
}

func (r Result) WriteText(w io.Writer) error {
	var msg string
	switch r.Action {
	case "loaded":
		msg = fmt.Sprintf("Loaded %d folders and %d documents under %s.\n"+
			"Run this query with the context of each demo user (see 'openarch credentials'), and compare:\n"+
			"  openarch query \"SELECT * FROM SysFile WHERE sys_path STARTSWITH '%s/'\" --context <context>\n",
			r.Folders, r.Documents, r.Root, r.Root)
	case "exists":
		msg = fmt.Sprintf("%s exists, nothing loaded. Use --force to load it again.\n", r.Root)
	case "deleted":
		msg = fmt.Sprintf("Deleted %s.\n", r.Root)
	case "absent":
		msg = fmt.Sprintf("%s does not exist, nothing deleted.\n", r.Root)
	}
	_, err := io.WriteString(w, msg)
	return err
}

// Load creates the demo folders and documents as an administrator. groups
// maps group names to ids, from the users file. With force, an existing Root
// is deleted first.
func Load(ctx context.Context, c Client, groups map[string]string, force bool) (Result, error) {
	res := Result{Root: Root}
	for _, f := range Folders {
		if f.Group == "" {
			continue
		}
		id, ok := groups[f.Group]
		if !ok {
			return res, output.Usage(fmt.Sprintf("group %s is not in %s", f.Group, users.Path),
				"Run 'openarch demo users', then 'openarch up'.")
		}
		known, err := c.GroupExists(ctx, id)
		if err != nil {
			return res, err
		}
		if !known {
			return res, output.Usage(fmt.Sprintf("OpenArch does not know group %s yet: it reads its users only at startup", f.Group),
				"Run 'openarch up' to restart OpenArch.")
		}
	}
	doc, err := c.GetDocument(ctx, Root)
	switch {
	case err == nil && !force:
		res.Action = "exists"
		return res, nil
	case err == nil:
		if err := c.DeleteDocument(ctx, doc.ID); err != nil {
			return res, err
		}
	case !errors.Is(err, api.ErrNotFound):
		return res, err
	}
	create := func(parent string, body map[string]any) error {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		d, err := c.CreateDocument(ctx, parent, data)
		if err != nil {
			return err
		}
		res.Created = append(res.Created, d.Path)
		return nil
	}
	name := strings.TrimPrefix(Root, "/")
	if err := create("/", map[string]any{"sys_primaryType": "SysFolder", "sys_name": name, "sys_title": name}); err != nil {
		return res, err
	}
	for _, f := range Folders {
		body := map[string]any{"sys_primaryType": "SysFolder", "sys_name": f.Name, "sys_title": f.Name}
		if f.Group != "" {
			// OpenArch needs the group id: an entry with only a name gives no access.
			body["sys_acl"] = []map[string]any{{"group": map[string]string{"id": groups[f.Group]}, "permission": "Read", "granted": true}}
		}
		if err := create(Root, body); err != nil {
			return res, err
		}
		res.Folders++
		for _, d := range f.Docs {
			if err := create(Root+"/"+f.Name, map[string]any{"sys_primaryType": "SysFile", "sys_name": d.Name,
				"sys_title": d.Title, "sys_description": d.Description}); err != nil {
				return res, err
			}
			res.Documents++
		}
	}
	res.Action = "loaded"
	return res, nil
}

// Reset deletes Root and everything in it.
func Reset(ctx context.Context, c Client) (Result, error) {
	res := Result{Root: Root, Action: "absent"}
	doc, err := c.GetDocument(ctx, Root)
	if errors.Is(err, api.ErrNotFound) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if err := c.DeleteDocument(ctx, doc.ID); err != nil {
		return res, err
	}
	res.Action = "deleted"
	return res, nil
}

// GroupIDs maps the group names of a users file to their ids.
func GroupIDs(f users.File) map[string]string {
	m := map[string]string{}
	for _, g := range f.Groups {
		m[g.Name] = g.ID
	}
	return m
}
