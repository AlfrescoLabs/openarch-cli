// Package generate creates a workspace (init) and writes its files again from
// openarch.yaml (render).
package generate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rogpeppe/go-internal/diff"
	"gopkg.in/yaml.v3"

	"github.com/AlfrescoLabs/openarch-cli/internal/certs"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/templates"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// DefaultAdminUser is the OpenArch admin user name.
const DefaultAdminUser = "admin"

// PasswordLength is the length of generated passwords.
const PasswordLength = 24

// File actions.
const (
	Create    = "create"
	Update    = "update"
	Unchanged = "unchanged"
	Delete    = "delete"
)

// Env is the access to the system. Tests replace it with fakes.
type Env struct {
	// HostMemory returns the memory available to Docker, in bytes.
	HostMemory func(ctx context.Context) (int64, error)
	Now        func() time.Time
}

// InitOptions are the inputs of Init.
type InitOptions struct {
	Dir    string
	Config workspace.Config
	// AdminPassword is the new admin password. Empty keeps the password of an
	// existing .env.local (with Force) or generates one.
	AdminPassword string
	Force         bool
	DryRun        bool
}

// RenderOptions are the inputs of Render.
type RenderOptions struct {
	Dir    string
	DryRun bool
}

// FileResult is what happened, or would happen, to one file.
type FileResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	// Diff is set by render --dry-run for updated files that hold no secrets.
	Diff string `json:"diff,omitempty"`
}

// Result is the outcome of Init or Render.
type Result struct {
	Command      string           `json:"-"`
	Workspace    string           `json:"workspace"`
	DryRun       bool             `json:"dryRun"`
	URL          string           `json:"url"`
	AdminUser    string           `json:"adminUser"`
	PasswordFile string           `json:"passwordFile"`
	Config       workspace.Config `json:"config"`
	Sizing       workspace.Sizing `json:"sizing"`
	Files        []FileResult     `json:"files"`
	Folders      []string         `json:"folders,omitempty"`
	// Warnings go to stderr, never to stdout.
	Warnings []string `json:"-"`
}

type plannedFile struct {
	templates.File
	secret bool // never shown in a diff
}

// Init creates a workspace in opts.Dir.
func Init(ctx context.Context, env Env, opts InitOptions) (Result, error) {
	cfg := opts.Config
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}
	if !opts.Force {
		for _, name := range []string{workspace.ConfigFile, "compose.yaml", localenv.File} {
			if exists(filepath.Join(opts.Dir, name)) {
				return Result{}, output.Usage(
					fmt.Sprintf("%s already has %s", opts.Dir, name),
					"Use --force to overwrite the workspace, or 'openarch render' to update it from openarch.yaml.")
			}
		}
	}
	secrets := localenv.Env{AdminUser: DefaultAdminUser, AdminPassword: opts.AdminPassword}
	if secrets.AdminPassword == "" {
		old, err := readLocalEnv(opts.Dir)
		if err == nil && old.AdminPassword != "" {
			secrets = old
		} else {
			secrets.AdminPassword = localenv.Password(PasswordLength)
		}
	}
	if secrets.AdminUser == "" {
		secrets.AdminUser = DefaultAdminUser
	}
	r := Result{Command: "init"}
	return r, run(ctx, env, opts.Dir, cfg, secrets, opts.DryRun, false, &r)
}

// Render writes the workspace files again from openarch.yaml and .env.local.
func Render(ctx context.Context, env Env, opts RenderOptions) (Result, error) {
	cfg, err := workspace.Load(filepath.Join(opts.Dir, workspace.ConfigFile))
	if err != nil {
		return Result{}, err
	}
	secrets, err := readLocalEnv(opts.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, output.Usage(localenv.File+" not found in "+opts.Dir,
			"Create it with "+localenv.AdminUserKey+" and "+localenv.AdminPasswordKey+", or run 'openarch init --force'.")
	}
	if err != nil {
		return Result{}, err
	}
	if secrets.AdminUser == "" || secrets.AdminPassword == "" {
		return Result{}, output.Usage(localenv.File+" has no admin user or password",
			"Set "+localenv.AdminUserKey+" and "+localenv.AdminPasswordKey+" in "+localenv.File+".")
	}
	r := Result{Command: "render"}
	return r, run(ctx, env, opts.Dir, cfg, secrets, opts.DryRun, true, &r)
}

func run(ctx context.Context, env Env, dir string, cfg workspace.Config, secrets localenv.Env, dryRun, showDiff bool, r *Result) error {
	r.Workspace, r.DryRun, r.Config = dir, dryRun, cfg
	r.URL, r.AdminUser, r.PasswordFile = cfg.URL(), secrets.AdminUser, localenv.File
	r.Warnings = cfg.Warnings()

	budget, err := workspace.ParseMemory(cfg.Memory)
	if err != nil {
		return err
	}
	if budget == 0 {
		budget, err = env.HostMemory(ctx)
		if err != nil || budget <= 0 {
			budget = workspace.FallbackMemory
			r.Warnings = append(r.Warnings, "Docker did not report its memory, so memory settings are sized for 4g. Use --memory to choose.")
		}
	}
	r.Sizing = workspace.SizingFor(budget)

	hash := existingHash(dir, secrets)
	if hash == "" {
		if hash, err = localenv.Hash(secrets.AdminPassword); err != nil {
			return err
		}
	}
	rendered, err := templates.Render(templates.Input{Config: cfg, Sizing: r.Sizing, AdminUser: secrets.AdminUser, AdminHash: hash})
	if err != nil {
		return err
	}
	planned := []plannedFile{{File: templates.File{Path: workspace.ConfigFile, Data: cfg.Marshal(), Mode: 0o644}}}
	for _, f := range rendered {
		planned = append(planned, plannedFile{File: f})
	}
	planned = append(planned, plannedFile{File: templates.File{Path: localenv.File, Data: secrets.Marshal(), Mode: 0o600}, secret: true})
	tlsFiles, warn, err := planTLS(dir, cfg, env.Now())
	if err != nil {
		return err
	}
	planned = append(planned, tlsFiles...)
	r.Warnings = append(r.Warnings, warn...)

	var deletes []string
	if !cfg.TLSEnabled() && exists(filepath.Join(dir, "config", "Caddyfile")) {
		deletes = append(deletes, "config/Caddyfile")
	}

	for _, f := range planned {
		fr, err := compare(dir, f, showDiff)
		if err != nil {
			return err
		}
		r.Files = append(r.Files, fr)
	}
	for _, p := range deletes {
		r.Files = append(r.Files, FileResult{Path: p, Action: Delete})
	}
	if cfg.Storage.Mode == workspace.StorageBind {
		for _, sub := range []string{"mongodb", "opensearch", "openarch"} {
			r.Folders = append(r.Folders, filepath.Join(dataDir(dir, cfg), sub))
		}
	}
	if dryRun {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, f := range planned {
		if r.Files[i].Action == Unchanged {
			continue
		}
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(f.Path)), f.Data, f.Mode); err != nil {
			return err
		}
	}
	for _, p := range deletes {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for _, folder := range r.Folders {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// planTLS returns the certificate files to write and checks custom certificates.
func planTLS(dir string, cfg workspace.Config, now time.Time) ([]plannedFile, []string, error) {
	switch cfg.TLS.Mode {
	case workspace.TLSSelfSigned:
		certPath := filepath.Join(dir, filepath.FromSlash(templates.SelfSignedCert))
		keyPath := filepath.Join(dir, filepath.FromSlash(templates.SelfSignedKey))
		certPEM, err1 := os.ReadFile(certPath)
		keyPEM, err2 := os.ReadFile(keyPath)
		if err1 != nil || err2 != nil || !certs.Reusable(certPEM, keyPEM, cfg.ServerName, now) {
			var err error
			certPEM, keyPEM, err = certs.SelfSigned(cfg.ServerName, now)
			if err != nil {
				return nil, nil, err
			}
		}
		return []plannedFile{
			{File: templates.File{Path: templates.SelfSignedCert, Data: certPEM, Mode: 0o644}},
			{File: templates.File{Path: templates.SelfSignedKey, Data: keyPEM, Mode: 0o600}, secret: true},
		}, nil, nil
	case workspace.TLSCustom:
		certFile, keyFile := resolve(dir, cfg.TLS.Cert), resolve(dir, cfg.TLS.Key)
		certPEM, err := os.ReadFile(certFile)
		if err != nil {
			return nil, nil, output.Usage("cannot read certificate: "+err.Error(), "Check --cert.")
		}
		keyPEM, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, nil, output.Usage("cannot read key: "+err.Error(), "Check --key.")
		}
		cert, err := certs.CheckPair(certPEM, keyPEM)
		if err != nil {
			return nil, nil, output.Usage("invalid TLS files: "+err.Error(), "--cert must be a PEM certificate and --key its PEM private key.")
		}
		var warn []string
		if cert.VerifyHostname(cfg.ServerName) != nil {
			warn = append(warn, fmt.Sprintf("The certificate is not valid for %s. Clients will reject it.", cfg.ServerName))
		}
		if now.After(cert.NotAfter) {
			warn = append(warn, "The certificate expired on "+cert.NotAfter.Format(time.DateOnly)+".")
		}
		return nil, warn, nil
	}
	return nil, nil, nil
}

// existingHash returns the admin hash in config/users.yml if it still matches
// the password, so that render does not change the file without a reason.
func existingHash(dir string, secrets localenv.Env) string {
	data, err := os.ReadFile(filepath.Join(dir, "config", "users.yml"))
	if err != nil {
		return ""
	}
	var f struct {
		Users []struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"users"`
	}
	if yaml.Unmarshal(data, &f) != nil {
		return ""
	}
	for _, u := range f.Users {
		if u.Username == secrets.AdminUser && localenv.Matches(u.Password, secrets.AdminPassword) {
			return u.Password
		}
	}
	return ""
}

func compare(dir string, f plannedFile, showDiff bool) (FileResult, error) {
	fr := FileResult{Path: f.Path}
	old, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fr.Action = Create
	case err != nil:
		return fr, err
	case string(old) == string(f.Data):
		fr.Action = Unchanged
	default:
		fr.Action = Update
		if showDiff && !f.secret {
			fr.Diff = string(diff.Diff("a/"+f.Path, old, "b/"+f.Path, f.Data))
		}
	}
	return fr, nil
}

// writeFile replaces path atomically, so a failed write never leaves a half-written file.
func writeFile(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func readLocalEnv(dir string) (localenv.Env, error) {
	data, err := os.ReadFile(filepath.Join(dir, localenv.File))
	if err != nil {
		return localenv.Env{}, err
	}
	return localenv.Parse(data), nil
}

func dataDir(dir string, cfg workspace.Config) string {
	return resolve(dir, cfg.Storage.DataDir)
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, filepath.FromSlash(p))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteText writes the human-readable form.
func (r Result) WriteText(w io.Writer) error {
	var b strings.Builder
	switch {
	case r.DryRun:
		fmt.Fprintf(&b, "Dry run: nothing was written to %s\n\n", r.Workspace)
	case r.Command == "init":
		fmt.Fprintf(&b, "Created workspace %s\n\n", r.Workspace)
	default:
		fmt.Fprintf(&b, "Rendered workspace %s\n\n", r.Workspace)
	}
	for _, f := range r.Files {
		fmt.Fprintf(&b, "  %-9s %s\n", f.Action, f.Path)
	}
	for _, f := range r.Folders {
		fmt.Fprintf(&b, "  %-9s %s/\n", "folder", f)
	}
	for _, f := range r.Files {
		if f.Diff != "" {
			fmt.Fprintf(&b, "\n%s", f.Diff)
		}
	}
	fmt.Fprintf(&b, "\nOpenArch URL: %s\n", r.URL)
	fmt.Fprintf(&b, "Admin user:   %s (password in %s)\n", r.AdminUser, r.PasswordFile)
	if !r.DryRun {
		fmt.Fprintf(&b, "\nStart it with: cd %s && docker compose up -d\n", r.Workspace)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
