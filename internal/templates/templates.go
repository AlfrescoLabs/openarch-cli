// Package templates renders the workspace files from openarch.yaml.
package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

//go:embed files/*.tmpl
var files embed.FS

// Images are the pinned images of the stack, except OpenArch, whose tag comes from openarch.yaml.
type Images struct {
	MongoDB    string
	OpenSearch string
	Dashboards string
	Caddy      string
}

// DefaultImages are the images tested with this CLI. MongoDB and OpenSearch match upstream OpenArch.
var DefaultImages = Images{
	MongoDB:    "mongo:7.0.39@sha256:9bdaeb6dac6e7e762e84e2f84103d1f9bb078fa1ba6bde8bb9d2274f655ad173",
	OpenSearch: "opensearchproject/opensearch:2.19.6@sha256:8690b204fe914c60ca76d451ac73bc0481e034d32d3779944c8caca56a2b003f",
	Dashboards: "opensearchproject/opensearch-dashboards:2.19.6@sha256:7aaefffac1660cab61ecf1e52fdcde6c156587a020dfa9246d529ea5e784dcee",
	Caddy:      "caddy:2.11.7@sha256:36c24ad91a7ff7bc4c87fe22bfa8df3c4b18da0773d73fc12ee2c52b98c0bb7e",
}

// Workspace paths of the files written for TLS self-signed.
const (
	SelfSignedCert = "certs/cert.pem"
	SelfSignedKey  = "certs/key.pem"
)

// Input is everything the templates need.
type Input struct {
	Config    workspace.Config
	Sizing    workspace.Sizing
	AdminUser string
	// AdminHash is the admin password in the users file format ({bcrypt}...).
	AdminHash string
}

// File is a rendered file. Path is relative to the workspace and uses '/'.
type File struct {
	Path string
	Data []byte
	Mode fs.FileMode
}

type data struct {
	Input
	Images           Images
	Volume, Bind     bool
	DataDir          string
	CertFile         string
	KeyFile          string
	HostIP           string
	GitignoreDataDir string
}

var funcs = template.FuncMap{
	// quote returns s as a YAML double-quoted string. JSON strings are valid YAML.
	"quote": func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	},
	// mount returns a quoted compose bind mount "host:target". The host path
	// is base/elem, with '$' escaped so that compose does not read it as a variable.
	"mount": func(base, elem, target string) string {
		p := base
		if elem != "" {
			p = strings.TrimSuffix(base, "/") + "/" + elem
		}
		b, _ := json.Marshal(strings.ReplaceAll(p, "$", "$$") + ":" + target)
		return string(b)
	},
	"size": func(n int64) string {
		if n%(1<<30) == 0 {
			return fmt.Sprintf("%dg", n>>30)
		}
		return fmt.Sprintf("%dm", n>>20)
	},
}

var tmpl = template.Must(template.New("").Funcs(funcs).Option("missingkey=error").ParseFS(files, "files/*.tmpl"))

// Render returns the generated files of a workspace, in a stable order.
// It does not include openarch.yaml, .env.local or certificates.
func Render(in Input) ([]File, error) {
	if in.AdminUser == "" || in.AdminHash == "" {
		return nil, fmt.Errorf("render: admin user and password hash are required")
	}
	c := in.Config
	d := data{
		Input:  in,
		Images: DefaultImages,
		Volume: c.Storage.Mode == workspace.StorageVolume,
		Bind:   c.Storage.Mode == workspace.StorageBind,
		HostIP: hostIP(c),
	}
	if d.Bind {
		d.DataDir = filepath.ToSlash(c.Storage.DataDir)
		d.GitignoreDataDir = gitignorePattern(d.DataDir)
	}
	switch c.TLS.Mode {
	case workspace.TLSSelfSigned:
		d.CertFile, d.KeyFile = "./"+SelfSignedCert, "./"+SelfSignedKey
	case workspace.TLSCustom:
		d.CertFile, d.KeyFile = filepath.ToSlash(c.TLS.Cert), filepath.ToSlash(c.TLS.Key)
	}

	out := []File{
		{Path: "compose.yaml", Mode: 0o644},
		{Path: ".env", Mode: 0o644},
		{Path: "config/users.yml", Mode: 0o644},
		{Path: ".gitignore", Mode: 0o644},
		{Path: "README.md", Mode: 0o644},
	}
	if c.TLSEnabled() {
		out = append(out, File{Path: "config/Caddyfile", Mode: 0o644})
	}
	for i := range out {
		var b bytes.Buffer
		if err := tmpl.ExecuteTemplate(&b, templateName(out[i].Path), d); err != nil {
			return nil, fmt.Errorf("render %s: %w", out[i].Path, err)
		}
		out[i].Data = b.Bytes()
	}
	return out, nil
}

func templateName(p string) string {
	return strings.TrimPrefix(path.Base(p), ".") + ".tmpl"
}

// hostIP is the address to show for the extra ports, which are published on BindIP.
func hostIP(c workspace.Config) string {
	ip := net.ParseIP(c.BindIP)
	switch {
	case ip == nil || ip.IsUnspecified():
		return c.ServerName
	case ip.IsLoopback() && c.ServerName == "localhost":
		return "localhost"
	case ip.To4() == nil:
		return "[" + c.BindIP + "]"
	}
	return c.BindIP
}

// gitignorePattern returns the .gitignore line for a data folder inside the
// workspace, or "" when the folder is outside it.
func gitignorePattern(dir string) string {
	if path.IsAbs(dir) || filepath.IsAbs(dir) {
		return ""
	}
	clean := path.Clean(dir)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return "/" + clean + "/"
}
