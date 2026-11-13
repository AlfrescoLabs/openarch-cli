// Package demo holds the sample data of 'openarch demo': three users, and
// folders and documents with different read access, so that one query returns
// different documents for each user. The data is in fixture.yaml.
//
// OpenArch reads its users only at startup, so the users are written to the
// users file before 'openarch up', and the documents are created after it.
package demo

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/users"
)

//go:embed fixture.yaml
var fixtureYAML []byte

// Fixture is the demo data, read from fixture.yaml.
type Fixture struct {
	Version     int      `yaml:"version"`
	Root        string   `yaml:"root"`
	Users       []User   `yaml:"users"`
	SharedWords []string `yaml:"sharedWords"`
	Folders     []Folder `yaml:"folders"`
	Documents   []Doc    `yaml:"documents"`
}

// User is a demo user and its groups. Every demo user is in the members group.
type User struct {
	Name   string   `yaml:"name"`
	Groups []string `yaml:"groups"`
}

// ACE is an access entry of the fixture. It names one user or one group.
type ACE struct {
	User       string `yaml:"user,omitempty"`
	Group      string `yaml:"group,omitempty"`
	Permission string `yaml:"permission"`
	// End is the time the entry stops giving access, in RFC 3339 form.
	End string `yaml:"end,omitempty"`
}

// Folder is a demo folder. A folder with no entries gives access only through
// the entries it inherits.
type Folder struct {
	Path             string `yaml:"path"`
	ACL              []ACE  `yaml:"acl"`
	BlockInheritance bool   `yaml:"blockInheritance"`
}

// Doc is a demo document. It has metadata only: OpenArch searches metadata,
// and does not extract text from files.
type Doc struct {
	Name             string   `yaml:"name"`
	Folder           string   `yaml:"folder"`
	Title            string   `yaml:"title"`
	Description      string   `yaml:"description"`
	Canary           string   `yaml:"canary"`
	ACL              []ACE    `yaml:"acl"`
	BlockInheritance bool     `yaml:"blockInheritance"`
	VisibleTo        []string `yaml:"visibleTo"`
}

// Path is the path of the document.
func (d Doc) Path() string { return d.Folder + "/" + d.Name }

// EmbeddingText is the text the vector of the document is made from: OpenArch
// has no text of the document other than its metadata.
func (d Doc) EmbeddingText() string { return d.Title + ". " + d.Description }

// Data is the demo data. fixture.yaml is part of the binary, so a parse error
// is a build error that the tests catch.
var Data = mustParse(fixtureYAML)

func mustParse(b []byte) Fixture {
	var f Fixture
	if err := yaml.Unmarshal(b, &f); err != nil {
		panic("demo: fixture.yaml: " + err.Error())
	}
	return f
}

// Users are the demo users.
var Users = Data.Users

// Root is the folder that holds the demo documents.
const Root = "/Demo"

// ExtraGroups returns the groups of u other than the members group. It is never
// nil, so JSON output has [] for a user with no extra group.
func (u User) ExtraGroups() []string {
	gs := []string{}
	for _, g := range u.Groups {
		if g != users.MembersGroup {
			gs = append(gs, g)
		}
	}
	return gs
}

// AddUser adds u to f, or updates it, with the password hash and the demo
// groups. It reports whether f changed.
func AddUser(f *users.File, u User, hash string) bool {
	for _, g := range u.Groups {
		f.EnsureGroup(g)
	}
	i := f.Find(u.Name)
	if i < 0 {
		f.Users = append(f.Users, users.User{ID: users.NewID(), Username: u.Name, Password: hash, Groups: slices.Clone(u.Groups)})
		return true
	}
	changed := f.Users[i].Password != hash
	f.Users[i].Password = hash
	for _, g := range u.Groups {
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
	if i < 0 {
		return false
	}
	for _, g := range u.Groups {
		if !slices.Contains(f.Users[i].Groups, g) {
			return false
		}
	}
	return true
}

// Client is the part of the API client that Load and Reset use.
type Client interface {
	GetDocument(ctx context.Context, ref string) (api.Document, error)
	CreateDocument(ctx context.Context, parent string, body []byte) (api.Document, error)
	DeleteDocument(ctx context.Context, id string) error
	GroupExists(ctx context.Context, id string) (bool, error)
	UserExists(ctx context.Context, id string) (bool, error)
	StoreEmbeddings(ctx context.Context, doc api.Document, typ string, rows []api.EmbeddingRow) error
}

// Embedder makes one vector per text.
type Embedder func(ctx context.Context, texts []string) ([][]float32, error)

// LoadOptions are the options of Load.
type LoadOptions struct {
	// Force deletes an existing Root first.
	Force bool
	// Embed, when set, makes a vector for each document, stored with type
	// EmbeddingType, so that a semantic query can find it.
	Embed         Embedder
	EmbeddingType string
}

// IDs maps the user and group names of the users file to their ids.
type IDs struct {
	Users  map[string]string
	Groups map[string]string
}

// Result is the result of Load and Reset.
type Result struct {
	// Action is "loaded", "exists", "deleted" or "absent".
	Action    string   `json:"action"`
	Root      string   `json:"root"`
	Folders   int      `json:"folders"`
	Documents int      `json:"documents"`
	Created   []string `json:"created,omitempty"`
	// Embeddings is the number of documents that got a vector.
	Embeddings    int    `json:"embeddings"`
	EmbeddingType string `json:"embeddingType,omitempty"`
}

func (r Result) WriteText(w io.Writer) error {
	var msg string
	switch r.Action {
	case "loaded":
		msg = fmt.Sprintf("Loaded %d folders and %d documents under %s.\n"+
			"Run this query with the context of each demo user (see 'openarch credentials'), and compare:\n"+
			"  openarch query \"SELECT * FROM SysFile WHERE sys_path STARTSWITH '%s/'\" --context <context>\n",
			r.Folders, r.Documents, r.Root, r.Root)
		if r.Embeddings > 0 {
			msg += fmt.Sprintf("Stored a vector of type %s for %d documents. A semantic query:\n"+
				"  openarch query --semantic \"How much is the pay for each job level?\" --context <context>\n",
				r.EmbeddingType, r.Embeddings)
		}
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

// Load creates the demo folders and documents as an administrator. ids comes
// from the users file.
func Load(ctx context.Context, c Client, ids IDs, opts LoadOptions) (Result, error) {
	res := Result{Root: Root}
	if err := checkPrincipals(ctx, c, ids); err != nil {
		return res, err
	}
	// The vectors are made first, so that a missing model changes nothing.
	var vectors [][]float32
	if opts.Embed != nil {
		texts := make([]string, len(Data.Documents))
		for i, d := range Data.Documents {
			texts[i] = d.EmbeddingText()
		}
		var err error
		if vectors, err = opts.Embed(ctx, texts); err != nil {
			return res, err
		}
		if len(vectors) != len(texts) {
			return res, fmt.Errorf("%d vectors for %d documents", len(vectors), len(texts))
		}
	}
	doc, err := c.GetDocument(ctx, Root)
	switch {
	case err == nil && !opts.Force:
		res.Action = "exists"
		return res, nil
	case err == nil:
		if err := c.DeleteDocument(ctx, doc.ID); err != nil {
			return res, err
		}
	case !errors.Is(err, api.ErrNotFound):
		return res, err
	}
	create := func(p string, body map[string]any) (api.Document, error) {
		data, err := json.Marshal(body)
		if err != nil {
			return api.Document{}, err
		}
		d, err := c.CreateDocument(ctx, path.Dir(p), data)
		if err != nil {
			return api.Document{}, err
		}
		res.Created = append(res.Created, d.Path)
		return d, nil
	}
	// Folders are listed parents first.
	for _, f := range Data.Folders {
		name := path.Base(f.Path)
		body := map[string]any{"sys_primaryType": "SysFolder", "sys_name": name, "sys_title": name}
		setACL(body, f.ACL, f.BlockInheritance, ids)
		if _, err := create(f.Path, body); err != nil {
			return res, err
		}
		if f.Path != Root {
			res.Folders++
		}
	}
	for i, d := range Data.Documents {
		body := map[string]any{"sys_primaryType": "SysFile", "sys_name": d.Name, "sys_title": d.Title, "sys_description": d.Description}
		setACL(body, d.ACL, d.BlockInheritance, ids)
		created, err := create(d.Path(), body)
		if err != nil {
			return res, err
		}
		res.Documents++
		if vectors != nil {
			// One vector per document, so the document id makes the row id unique.
			row := api.EmbeddingRow{ID: created.ID + "/0", Text: d.EmbeddingText(), Vector: vectors[i]}
			if err := c.StoreEmbeddings(ctx, created, opts.EmbeddingType, []api.EmbeddingRow{row}); err != nil {
				return res, err
			}
			res.Embeddings++
			res.EmbeddingType = opts.EmbeddingType
		}
	}
	res.Action = "loaded"
	return res, nil
}

// checkPrincipals fails when a user or group of an access entry is missing
// from the users file, or when OpenArch does not know it yet.
func checkPrincipals(ctx context.Context, c Client, ids IDs) error {
	seen := map[string]bool{}
	var aces []ACE
	for _, f := range Data.Folders {
		aces = append(aces, f.ACL...)
	}
	for _, d := range Data.Documents {
		aces = append(aces, d.ACL...)
	}
	for _, a := range aces {
		kind, name, m, exists := "group", a.Group, ids.Groups, c.GroupExists
		if a.User != "" {
			kind, name, m, exists = "user", a.User, ids.Users, c.UserExists
		}
		if seen[kind+" "+name] {
			continue
		}
		seen[kind+" "+name] = true
		id, ok := m[name]
		if !ok {
			return output.Usage(fmt.Sprintf("%s %s is not in %s", kind, name, users.Path),
				"Run 'openarch demo users', then 'openarch up'.")
		}
		known, err := exists(ctx, id)
		if err != nil {
			return err
		}
		if !known {
			return output.Usage(fmt.Sprintf("OpenArch does not know %s %s yet: it reads its users only at startup", kind, name),
				"Run 'openarch up' to restart OpenArch.")
		}
	}
	return nil
}

// setACL sets sys_acl in body from the fixture entries. OpenArch needs the
// user or group id: an entry with only a name gives no access. No entries and
// no block leave sys_acl out, so the item only inherits.
func setACL(body map[string]any, aces []ACE, block bool, ids IDs) {
	var acl []map[string]any
	for _, a := range aces {
		e := map[string]any{"permission": a.Permission, "granted": true}
		if a.User != "" {
			e["user"] = map[string]string{"id": ids.Users[a.User]}
		} else {
			e["group"] = map[string]string{"id": ids.Groups[a.Group]}
		}
		if a.End != "" {
			e["end"] = a.End
		}
		acl = append(acl, e)
	}
	if block {
		// The only entry that denies Read: it must come last, after the grants.
		acl = append(acl, map[string]any{"user": map[string]string{"id": everyone}, "permission": "Everything", "granted": false})
	}
	if acl != nil {
		body["sys_acl"] = acl
	}
}

// everyone is the OpenArch principal that every user has.
const everyone = "__Everyone__"

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

// IDsOf maps the user and group names of a users file to their ids.
func IDsOf(f users.File) IDs {
	ids := IDs{Users: map[string]string{}, Groups: map[string]string{}}
	for _, g := range f.Groups {
		ids.Groups[g.Name] = g.ID
	}
	for _, u := range f.Users {
		ids.Users[u.Username] = u.ID
	}
	return ids
}
