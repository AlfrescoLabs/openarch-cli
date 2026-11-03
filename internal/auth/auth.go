// Package auth finds the OpenArch URL and credentials of a command.
//
// For each value, the first source that has one wins: flags, then environment
// variables, then the workspace, then a hidden prompt. Workspace credentials
// are used only for the workspace URL, so they are never sent to another server.
package auth

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/templates"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Environment variables.
const (
	EnvURL      = "OPENARCH_URL"
	EnvUser     = "OPENARCH_USER"
	EnvPassword = "OPENARCH_PASSWORD"
	EnvInsecure = "OPENARCH_INSECURE"
)

// Source tells where a value came from.
type Source string

// Sources.
const (
	SourceFlag      Source = "flag"
	SourceEnv       Source = "env"
	SourceWorkspace Source = "workspace"
	SourcePrompt    Source = "prompt"
	SourceDefault   Source = "default"
)

// Sources records where each value came from. It never holds a secret.
type Sources struct {
	URL      Source `json:"url"`
	User     Source `json:"user"`
	Password Source `json:"password"`
}

// Credentials is a resolved connection.
type Credentials struct {
	URL      string
	User     string
	Password string
	Insecure bool
	Sources  Sources
	// Workspace is the workspace folder, when the URL came from it.
	Workspace string
	// DialAddr, when set, is the host:port to connect to instead of the URL host.
	// A workspace URL can use a server name that does not resolve on this machine.
	DialAddr string
	// RootCAPEM is an extra trusted certificate, such as a self-signed workspace certificate.
	RootCAPEM []byte
}

// Input holds the raw values of a command.
type Input struct {
	URL           string
	User          string
	Password      string // from --password
	PasswordStdin bool
	Insecure      bool
	InsecureSet   bool
	Stdin         io.Reader
	Getenv        func(string) string
	// WorkspaceDir is the folder to look for openarch.yaml in. Empty means no workspace.
	WorkspaceDir string
	// Prompt asks for a value. Nil when the command cannot prompt.
	Prompt func(label string, secret bool) (string, error)
}

// Resolve finds the URL, user and password.
func Resolve(in Input) (Credentials, error) {
	getenv := in.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	var c Credentials

	// URL
	switch {
	case in.URL != "":
		c.URL, c.Sources.URL = in.URL, SourceFlag
	case getenv(EnvURL) != "":
		c.URL, c.Sources.URL = getenv(EnvURL), SourceEnv
	}
	var ws *workspaceInfo
	if c.URL == "" && in.WorkspaceDir != "" {
		info, err := loadWorkspace(in.WorkspaceDir)
		if err != nil {
			return c, err
		}
		if info != nil {
			ws = info
			c.URL, c.Sources.URL = info.cfg.URL(), SourceWorkspace
			c.Workspace, c.DialAddr, c.RootCAPEM = info.dir, info.dialAddr(), info.rootCA
		}
	}
	if c.URL == "" {
		return c, output.Usage("no OpenArch URL",
			"Run the command inside a workspace, use --dir, or set --url or "+EnvURL+".")
	}
	c.URL = strings.TrimSuffix(c.URL, "/")
	if !strings.HasPrefix(c.URL, "http://") && !strings.HasPrefix(c.URL, "https://") {
		return c, output.Usage(fmt.Sprintf("invalid URL %q", c.URL), "Use a URL such as http://localhost:8080.")
	}

	// User
	switch {
	case in.User != "":
		c.User, c.Sources.User = in.User, SourceFlag
	case getenv(EnvUser) != "":
		c.User, c.Sources.User = getenv(EnvUser), SourceEnv
	case ws != nil && ws.secrets.AdminUser != "":
		c.User, c.Sources.User = ws.secrets.AdminUser, SourceWorkspace
	case in.Prompt != nil:
		u, err := in.Prompt("User: ", false)
		if err != nil {
			return c, err
		}
		c.User, c.Sources.User = strings.TrimSpace(u), SourcePrompt
	}
	if c.User == "" {
		return c, output.NewError(output.ExitAuth, "no OpenArch user", "Set --user or "+EnvUser+".")
	}

	// Password
	switch {
	case in.PasswordStdin:
		pw, err := readLine(in.Stdin)
		if err != nil {
			return c, err
		}
		c.Password, c.Sources.Password = pw, SourceFlag
	case in.Password != "":
		c.Password, c.Sources.Password = in.Password, SourceFlag
	case getenv(EnvPassword) != "":
		c.Password, c.Sources.Password = getenv(EnvPassword), SourceEnv
	case ws != nil && ws.secrets.AdminPassword != "" && c.User == ws.secrets.AdminUser:
		c.Password, c.Sources.Password = ws.secrets.AdminPassword, SourceWorkspace
	case in.Prompt != nil:
		pw, err := in.Prompt(fmt.Sprintf("Password for %s at %s: ", c.User, c.URL), true)
		if err != nil {
			return c, err
		}
		c.Password, c.Sources.Password = pw, SourcePrompt
	}
	if c.Password == "" {
		return c, output.NewError(output.ExitAuth, "no password for "+c.User+" at "+c.URL,
			"Use --password-stdin or "+EnvPassword+", or run in a terminal to be asked.")
	}

	// Insecure
	if in.InsecureSet {
		c.Insecure = in.Insecure
	} else if v := getenv(EnvInsecure); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, output.Usage(fmt.Sprintf("invalid %s: %q", EnvInsecure, v), "Use true or false.")
		}
		c.Insecure = b
	}
	return c, nil
}

// Describe says where the credentials came from, for error hints. It never shows the password.
func (c Credentials) Describe() string {
	switch c.Sources.Password {
	case SourceWorkspace:
		return fmt.Sprintf("user %s and password from the workspace %s (%s)", c.User, c.Workspace, localenv.File)
	case SourceEnv:
		return fmt.Sprintf("user %s (%s), password from %s", c.User, c.Sources.User, EnvPassword)
	case SourcePrompt:
		return fmt.Sprintf("user %s, password typed at the prompt", c.User)
	default:
		return fmt.Sprintf("user %s (%s), password from a flag", c.User, c.Sources.User)
	}
}

func readLine(r io.Reader) (string, error) {
	if r == nil {
		return "", output.Usage("--password-stdin needs a password on stdin", "")
	}
	data, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	pw := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if pw == "" {
		return "", output.Usage("the password from stdin is empty", "")
	}
	return pw, nil
}

type workspaceInfo struct {
	dir     string
	cfg     workspace.Config
	secrets localenv.Env
	rootCA  []byte
}

// loadWorkspace returns nil, nil when dir has no openarch.yaml.
func loadWorkspace(dir string) (*workspaceInfo, error) {
	path := filepath.Join(dir, workspace.ConfigFile)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	cfg, err := workspace.Load(path)
	if err != nil {
		return nil, err
	}
	info := &workspaceInfo{dir: dir, cfg: cfg}
	if data, err := os.ReadFile(filepath.Join(dir, localenv.File)); err == nil {
		info.secrets = localenv.Parse(data)
	}
	switch cfg.TLS.Mode {
	case workspace.TLSSelfSigned:
		info.rootCA, _ = os.ReadFile(filepath.Join(dir, filepath.FromSlash(templates.SelfSignedCert)))
	case workspace.TLSCustom:
		p := cfg.TLS.Cert
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		info.rootCA, _ = os.ReadFile(p)
	}
	return info, nil
}

// dialAddr is the published port on this machine.
func (w *workspaceInfo) dialAddr() string {
	host := w.cfg.BindIP
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(w.cfg.Port))
}
