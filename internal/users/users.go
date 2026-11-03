// Package users reads and writes the OpenArch users file (config/users.yml).
//
// OpenArch has no API to manage users: it reads this file once, at startup.
// The admin entry belongs to the workspace (its password is in .env.local).
// Other users and groups are kept as they are when the file is written again.
package users

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Path is the users file in a workspace.
const Path = "config/users.yml"

// Fixed ids, the same as in the users file bundled in the OpenArch image.
const (
	AdminID              = "00000000-0000-0000-0000-000000000001"
	AdministratorsGroup  = "administrators"
	MembersGroup         = "members"
	administratorsID     = "5324ec90-ba1e-4030-8e6a-d815c6f3d4b1"
	membersID            = "8d6542c9-50e9-4027-aa10-2cfb2179413f"
	bcryptPrefix         = "{bcrypt}"
	maxUsernameLength    = 64
	defaultAdminEmail    = "admin@example.com"
	defaultAdminFirst    = "Admin"
	defaultAdminLastName = "User"
)

// Group is a group of the users file.
type Group struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// User is a user of the users file. Password is "{bcrypt}<hash>".
type User struct {
	ID         string            `yaml:"id" json:"id"`
	Username   string            `yaml:"username" json:"username"`
	Password   string            `yaml:"password" json:"-"`
	FirstName  string            `yaml:"firstName,omitempty" json:"firstName,omitempty"`
	LastName   string            `yaml:"lastName,omitempty" json:"lastName,omitempty"`
	Email      string            `yaml:"email,omitempty" json:"email,omitempty"`
	Groups     []string          `yaml:"groups" json:"groups"`
	Attributes map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

// File is the content of the users file.
type File struct {
	AdministratorsGroups []string `yaml:"administratorsGroups"`
	Groups               []Group  `yaml:"groups"`
	Users                []User   `yaml:"users"`
}

// IsAdmin reports whether u is in an administrators group.
func (f File) IsAdmin(u User) bool {
	for _, g := range u.Groups {
		if slices.Contains(f.AdministratorsGroups, g) {
			return true
		}
	}
	return false
}

// Find returns the index of a user, or -1.
func (f File) Find(username string) int {
	return slices.IndexFunc(f.Users, func(u User) bool { return u.Username == username })
}

// Hash returns the users file form of a password hash.
func Hash(password string) (string, error) {
	h, err := localenv.Hash(password)
	if err != nil {
		return "", err
	}
	return bcryptPrefix + h, nil
}

// HashOf returns the bcrypt hash of a users file password, or "" when it is not bcrypt.
func HashOf(password string) string {
	h, _ := strings.CutPrefix(password, bcryptPrefix)
	if h == password {
		return ""
	}
	return h
}

// Build returns the users file with the workspace admin, plus the other
// users and groups of old (which may be the zero File).
func Build(adminUser, adminHash string, old File) File {
	f := File{
		AdministratorsGroups: []string{AdministratorsGroup},
		Groups:               []Group{{ID: administratorsID, Name: AdministratorsGroup}, {ID: membersID, Name: MembersGroup}},
	}
	admin := User{ID: AdminID, Username: adminUser, Password: bcryptPrefix + adminHash,
		FirstName: defaultAdminFirst, LastName: defaultAdminLastName, Email: defaultAdminEmail,
		Groups: []string{AdministratorsGroup, MembersGroup}}
	for _, g := range old.Groups {
		if g.Name != AdministratorsGroup && g.Name != MembersGroup {
			f.Groups = append(f.Groups, g)
		}
	}
	for _, g := range old.AdministratorsGroups {
		if !slices.Contains(f.AdministratorsGroups, g) {
			f.AdministratorsGroups = append(f.AdministratorsGroups, g)
		}
	}
	f.Users = append(f.Users, admin)
	for _, u := range old.Users {
		if u.ID == AdminID {
			// Keep the details the user may have edited, but not the name or password.
			f.Users[0].FirstName, f.Users[0].LastName, f.Users[0].Email = u.FirstName, u.LastName, u.Email
			f.Users[0].Attributes = u.Attributes
			continue
		}
		if u.Username == adminUser {
			continue
		}
		f.Users = append(f.Users, u)
	}
	return f
}

// Marshal encodes f with a header comment.
func (f File) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString(`# OpenArch users. OpenArch reads this file at startup: after a change, run 'openarch up'.
# Passwords are bcrypt hashes. The admin password is in .env.local.
# Manage users with 'openarch users add|set-password|delete'. 'openarch render'
# keeps every user and group, and writes the admin entry from .env.local.
`)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		panic(err) // File holds only strings, slices and maps.
	}
	return b.Bytes()
}

// Parse decodes a users file.
func Parse(data []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, output.Usage("cannot parse "+Path+": "+err.Error(), "Fix the file, or run 'openarch render' to write it again.")
	}
	return f, nil
}

// Load reads the users file of a workspace. A missing file is the zero File.
func Load(dir string) (File, error) {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(Path)))
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	return Parse(data)
}

// Save writes the users file of a workspace. It is not secret: it holds only hashes.
func Save(dir string, f File) error {
	p := filepath.Join(dir, filepath.FromSlash(Path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, f.Marshal(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

var validUsername = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]*$`)

// ValidateUsername checks a new user name.
func ValidateUsername(name string) error {
	if len(name) > maxUsernameLength || !validUsername.MatchString(name) {
		return output.Usage(fmt.Sprintf("invalid user name %q", name), "Use letters, digits, '.', '_', '@' and '-'.")
	}
	return nil
}

// NewID returns a random UUID (version 4).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error.
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// EnsureGroup adds a group with a new id when f does not have it.
func (f *File) EnsureGroup(name string) {
	if !slices.ContainsFunc(f.Groups, func(g Group) bool { return g.Name == name }) {
		f.Groups = append(f.Groups, Group{ID: NewID(), Name: name})
	}
}
