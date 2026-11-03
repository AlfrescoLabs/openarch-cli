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

// Keys in .env.local.
const (
	AdminUserKey     = "OPENARCH_ADMIN_USER"
	AdminPasswordKey = "OPENARCH_ADMIN_PASSWORD"
)

// alphabet has no look-alike characters and nothing that needs quoting in shells or .env files.
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

// Hash returns the password in the Spring Security format used by the OpenArch users file.
func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return "{bcrypt}" + string(h), nil
}

// Matches reports whether hash, in the users file format, matches password.
func Matches(hash, password string) bool {
	h, ok := strings.CutPrefix(hash, "{bcrypt}")
	return ok && bcrypt.CompareHashAndPassword([]byte(h), []byte(password)) == nil
}

// Env is the content of .env.local.
type Env struct {
	AdminUser     string
	AdminPassword string
}

// Marshal encodes e as a .env file.
func (e Env) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString("# Secrets of this OpenArch workspace. Keep this file private.\n")
	b.WriteString("# config/users.yml holds only a hash of the password.\n")
	b.WriteString("# After a change here, run 'openarch render' and restart OpenArch.\n")
	fmt.Fprintf(&b, "%s=%s\n", AdminUserKey, e.AdminUser)
	fmt.Fprintf(&b, "%s=%s\n", AdminPasswordKey, e.AdminPassword)
	return b.Bytes()
}

// Parse reads the keys this CLI uses from a .env file. Other lines are ignored.
func Parse(data []byte) Env {
	var e Env
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
		switch strings.TrimSpace(k) {
		case AdminUserKey:
			e.AdminUser = v
		case AdminPasswordKey:
			e.AdminPassword = v
		}
	}
	return e
}
