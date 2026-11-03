// Package localenv generates passwords and reads and writes .env.local, the
// file that holds the secrets of a workspace.
package localenv

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// File is the name of the secrets file in a workspace.
const File = ".env.local"

// PasswordLength is the length of generated passwords.
const PasswordLength = 24

// Keys in .env.local. Docker Compose reads them as secret sources.
const (
	AdminUserKey               = "OPENARCH_ADMIN_USER"
	AdminPasswordKey           = "OPENARCH_ADMIN_PASSWORD"
	MongoRootPasswordKey       = "MONGODB_ROOT_PASSWORD"
	MongoAppPasswordKey        = "MONGODB_APP_PASSWORD"
	MongoAppURIKey             = "MONGODB_APP_URI"
	OpenSearchAdminPasswordKey = "OPENSEARCH_ADMIN_PASSWORD"
	OpenSearchAppPasswordKey   = "OPENSEARCH_APP_PASSWORD"
	DashboardsPasswordKey      = "OPENSEARCH_DASHBOARDS_PASSWORD"
)

// Users that the CLI creates in MongoDB and OpenSearch.
const (
	MongoRootUser       = "root"
	MongoAppUser        = "openarch"
	MongoAppDB          = "openarch"
	OpenSearchAdminUser = "admin"
	OpenSearchAppUser   = "openarch"
	// The OpenSearch security plugin gives the Dashboards server rights only to
	// the user named in its server_username setting, which defaults to kibanaserver.
	DashboardsUser = "kibanaserver"
)

// alphabet has no look-alike characters and nothing that needs quoting in
// shells, .env files or URLs.
const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// Password returns a random password of n characters.
func Password(n int) string {
	out := make([]byte, 0, n)
	buf := make([]byte, 1)
	for len(out) < n {
		_, _ = rand.Read(buf) // crypto/rand.Read never returns an error.
		// Reject values above the largest multiple of len(alphabet) to avoid bias.
		if int(buf[0]) >= 256-256%len(alphabet) {
			continue
		}
		out = append(out, alphabet[int(buf[0])%len(alphabet)])
	}
	return string(out)
}

// Hash returns a bcrypt hash of password.
func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// Matches reports whether a bcrypt hash matches password.
func Matches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Env is the content of .env.local.
type Env struct {
	AdminUser               string
	AdminPassword           string
	MongoRootPassword       string
	MongoAppPassword        string
	OpenSearchAdminPassword string
	OpenSearchAppPassword   string
	DashboardsPassword      string
}

// fields maps each key to its field, in file order.
func (e *Env) fields() []struct {
	key string
	ptr *string
} {
	return []struct {
		key string
		ptr *string
	}{
		{AdminUserKey, &e.AdminUser},
		{AdminPasswordKey, &e.AdminPassword},
		{MongoRootPasswordKey, &e.MongoRootPassword},
		{MongoAppPasswordKey, &e.MongoAppPassword},
		{OpenSearchAdminPasswordKey, &e.OpenSearchAdminPassword},
		{OpenSearchAppPasswordKey, &e.OpenSearchAppPassword},
		{DashboardsPasswordKey, &e.DashboardsPassword},
	}
}

// Fill sets every empty value: the admin user to adminUser and the passwords to new random ones.
// It returns the keys it set.
func (e *Env) Fill(adminUser string) []string {
	var set []string
	for _, f := range e.fields() {
		if *f.ptr != "" {
			continue
		}
		if f.key == AdminUserKey {
			*f.ptr = adminUser
		} else {
			*f.ptr = Password(PasswordLength)
		}
		set = append(set, f.key)
	}
	return set
}

// MongoAppURI is the MongoDB connection string of OpenArch.
func (e Env) MongoAppURI() string {
	return fmt.Sprintf("mongodb://%s:%s@mongo:27017/?authSource=%s", MongoAppUser, e.MongoAppPassword, MongoAppDB)
}

// Marshal encodes e as a .env file.
func (e Env) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString(`# Secrets of this OpenArch workspace. Keep this file private (mode 0600, git-ignored).
# Docker Compose reads it as the source of its secrets, so run it with:
#   docker compose --env-file .env --env-file .env.local up -d
# Containers get the values as files in /run/secrets, never as environment variables.
#
# After a change of the OpenArch admin password or the MongoDB app password, run
# 'openarch render', then 'docker compose ... up -d --force-recreate'.
# The MongoDB root password and the OpenSearch passwords apply only when the data is
# created, at the first start. Changing them later here has no effect on existing data.
`)
	for _, f := range e.fields() {
		fmt.Fprintf(&b, "%s=%s\n", f.key, *f.ptr)
	}
	b.WriteString("# Built from MONGODB_APP_PASSWORD by 'openarch render'.\n")
	fmt.Fprintf(&b, "%s=%s\n", MongoAppURIKey, e.MongoAppURI())
	return b.Bytes()
}

// Parse reads the keys this CLI uses from a .env file. Other lines are ignored.
func Parse(data []byte) Env {
	var e Env
	fields := e.fields()
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		for _, f := range fields {
			if f.key == strings.TrimSpace(k) {
				*f.ptr = v
			}
		}
	}
	return e
}
