// Package auth finds the OpenArch URL and credentials of a command.
//
// For each value, the first source that has one wins: flags, then environment
// variables, then a base connection, then a hidden prompt. The base connection
// is a context chosen with --context or OPENARCH_CONTEXT, else the workspace
// found with --dir or in the current folder, else the current context.
// Credentials of the base connection are used only for its own URL, so they
// are never sent to another server.
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

	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
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
	SourceContext   Source = "context"
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
	// Context is the name of the context used, if any.
	Context string
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
	// Context is the --context flag. The OPENARCH_CONTEXT variable is read with Getenv.
	Context string
	// Contexts is the CLI config. Nil means no contexts.
	Contexts *contexts.Config
	// Store opens the credential store, for passwords of remote contexts.
	Store func() (credstore.Store, error)
	// AllowNoPassword returns the credentials even when no password is found.
	AllowNoPassword bool
}

// base is the connection of a context or a workspace.
type base struct {
	source    Source
	url       string
	user      string
	password  string
	insecure  bool
	workspace string
	context   string
	dialAddr  string
	rootCA    []byte
	// lookup reads the password of a remote context from the store, for a given user.
	lookup func(user string) (string, error)
}

// Resolve finds the URL, user and password.
func Resolve(in Input) (Credentials, error) {
	getenv := in.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	var c Credentials
	b, err := findBase(in, getenv)
	if err != nil {
		return c, err
	}

	// URL
	switch {
	case in.URL != "":
		c.URL, c.Sources.URL = in.URL, SourceFlag
	case getenv(EnvURL) != "":
		c.URL, c.Sources.URL = getenv(EnvURL), SourceEnv
	case b != nil:
		c.URL, c.Sources.URL = b.url, b.source
		c.Workspace, c.Context, c.DialAddr, c.RootCAPEM = b.workspace, b.context, b.dialAddr, b.rootCA
	}
	if c.URL == "" {
		return c, output.Usage("no OpenArch URL",
			"Run the command inside a workspace, use --dir or --context, or set --url or "+EnvURL+".")
	}
	c.URL = strings.TrimSuffix(c.URL, "/")
	if !strings.HasPrefix(c.URL, "http://") && !strings.HasPrefix(c.URL, "https://") {
		return c, output.Usage(fmt.Sprintf("invalid URL %q", c.URL), "Use a URL such as http://localhost:8080.")
	}
	// The base credentials belong to the base URL only.
	if b != nil && c.Sources.URL != b.source {
		b = nil
	}

	// User
	switch {
	case in.User != "":
		c.User, c.Sources.User = in.User, SourceFlag
	case getenv(EnvUser) != "":
		c.User, c.Sources.User = getenv(EnvUser), SourceEnv
	case b != nil && b.user != "":
		c.User, c.Sources.User = b.user, b.source
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
	case b != nil && b.password != "" && c.User == b.user:
		c.Password, c.Sources.Password = b.password, b.source
	case b != nil && b.lookup != nil:
		pw, err := b.lookup(c.User)
		if err != nil && !errors.Is(err, credstore.ErrNotFound) {
			return c, output.NewError(output.ExitAuth, "cannot read the stored password: "+err.Error(), "")
		}
		if pw != "" {
			c.Password, c.Sources.Password = pw, b.source
		}
	}
	if c.Password == "" && in.Prompt != nil {
		pw, err := in.Prompt(fmt.Sprintf("Password for %s at %s: ", c.User, c.URL), true)
		if err != nil {
			return c, err
		}
		c.Password, c.Sources.Password = pw, SourcePrompt
	}
	if c.Password == "" && !in.AllowNoPassword {
		return c, output.NewError(output.ExitAuth, "no password for "+c.User+" at "+c.URL,
			"Use --password-stdin or "+EnvPassword+", save it with 'openarch config set', or run in a terminal to be asked.")
	}

	// Insecure
	switch {
	case in.InsecureSet:
		c.Insecure = in.Insecure
	case getenv(EnvInsecure) != "":
		v, err := strconv.ParseBool(getenv(EnvInsecure))
		if err != nil {
			return c, output.Usage(fmt.Sprintf("invalid %s: %q", EnvInsecure, getenv(EnvInsecure)), "Use true or false.")
		}
		c.Insecure = v
	case b != nil:
		c.Insecure = b.insecure
	}
	return c, nil
}

// findBase returns the base connection, or nil when there is none.
func findBase(in Input, getenv func(string) string) (*base, error) {
	name, explicit := in.Context, in.Context != ""
	if name == "" {
		name, explicit = getenv(contexts.EnvContext), getenv(contexts.EnvContext) != ""
	}
	if !explicit && in.WorkspaceDir != "" {
		info, err := loadWorkspace(in.WorkspaceDir)
		if err != nil || info != nil {
			return info.base(SourceWorkspace, ""), err
		}
	}
	if !explicit && in.Contexts != nil {
		name = in.Contexts.CurrentContext
	}
	if name == "" {
		return nil, nil
	}
	cfg := in.Contexts
	if cfg == nil {
		cfg = &contexts.Config{}
	}
	ctx, err := cfg.Get(name)
	if err != nil {
		return nil, err
	}
	if ctx.Workspace != "" {
		info, err := loadWorkspace(ctx.Workspace)
		if err != nil {
			return nil, err
		}
		if info == nil {
			return nil, output.Usage(fmt.Sprintf("context %q points to %s, which has no %s", name, ctx.Workspace, workspace.ConfigFile),
				"Delete it with 'openarch config delete "+name+"'.")
		}
		return info.base(SourceContext, name), nil
	}
	ep, ok := ctx.Endpoints[contexts.SystemOpenArch]
	if !ok {
		return nil, output.Usage(fmt.Sprintf("context %q has no OpenArch endpoint", name), "Add one with 'openarch config set --name "+name+"'.")
	}
	b := &base{source: SourceContext, context: name, url: ep.URL, user: ep.Username, insecure: ep.Insecure}
	if in.Store != nil {
		url := strings.TrimSuffix(ep.URL, "/")
		b.lookup = func(user string) (string, error) {
			st, err := in.Store()
			if err != nil {
				return "", err
			}
			return st.Get(url, user)
		}
	}
	return b, nil
}

// Describe says where the credentials came from, for error hints. It never shows the password.
func (c Credentials) Describe() string {
	switch c.Sources.Password {
	case SourceWorkspace:
		return fmt.Sprintf("user %s and password from the workspace %s (%s)", c.User, c.Workspace, localenv.File)
	case SourceContext:
		if c.Workspace != "" {
			return fmt.Sprintf("user %s and password from context %q (workspace %s)", c.User, c.Context, c.Workspace)
		}
		return fmt.Sprintf("user %s and the stored password of context %q", c.User, c.Context)
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

func (w *workspaceInfo) base(source Source, context string) *base {
	if w == nil {
		return nil
	}
	return &base{source: source, context: context, workspace: w.dir, url: w.cfg.URL(),
		user: w.secrets.AdminUser, password: w.secrets.AdminPassword, dialAddr: w.dialAddr(), rootCA: w.rootCA}
}

// dialAddr is the published port on this machine.
func (w *workspaceInfo) dialAddr() string {
	host := w.cfg.BindIP
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(w.cfg.Port))
}
