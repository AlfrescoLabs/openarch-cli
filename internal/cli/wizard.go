package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/pflag"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

const envAccessible = "OPENARCH_ACCESSIBLE"

// question is one wizard prompt. Each question sets exactly one init flag.
type question struct {
	flag string
	// skip reports whether the question does not apply to the answers so far.
	skip  func(w *wizard) bool
	field func(w *wizard) huh.Field
}

// wizard holds the answers. Strings back the inputs that are numbers in the config.
type wizard struct {
	f        *initFlags
	port     string
	password string
}

var questions = []question{
	{flag: "storage", field: func(w *wizard) huh.Field {
		return huh.NewSelect[string]().Title("Storage").
			Description("Where OpenArch keeps its data.").
			Options(
				huh.NewOption("volume: Docker named volumes (recommended)", workspace.StorageVolume),
				huh.NewOption("bind: folders on this machine", workspace.StorageBind),
				huh.NewOption("none: data is lost when the containers are removed", workspace.StorageNone),
			).Value(&w.f.cfg.Storage.Mode)
	}},
	{flag: "data-dir", skip: func(w *wizard) bool { return w.f.cfg.Storage.Mode != workspace.StorageBind }, field: func(w *wizard) huh.Field {
		if w.f.cfg.Storage.DataDir == "" {
			w.f.cfg.Storage.DataDir = workspace.DefaultDataDir
		}
		return huh.NewInput().Title("Data folder").
			Description("Relative paths are relative to the workspace.").
			Value(&w.f.cfg.Storage.DataDir)
	}},
	{flag: "tls", field: func(w *wizard) huh.Field {
		return huh.NewSelect[string]().Title("TLS").
			Description("HTTPS through a Caddy proxy in front of OpenArch.").
			Options(
				huh.NewOption("none: plain HTTP", workspace.TLSNone),
				huh.NewOption("self-signed: the CLI creates a certificate", workspace.TLSSelfSigned),
				huh.NewOption("custom: use your own certificate files", workspace.TLSCustom),
			).Value(&w.f.cfg.TLS.Mode)
	}},
	{flag: "cert", skip: notCustomTLS, field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("Certificate file (PEM)").Value(&w.f.cfg.TLS.Cert).Validate(fileExists)
	}},
	{flag: "key", skip: notCustomTLS, field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("Private key file (PEM)").Value(&w.f.cfg.TLS.Key).Validate(fileExists)
	}},
	{flag: "server-name", field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("Server name").
			Description("Host name used in URLs and in the certificate.").
			Value(&w.f.cfg.ServerName)
	}},
	{flag: "port", field: func(w *wizard) huh.Field {
		if w.port == "" {
			w.port = strconv.Itoa(workspace.DefaultPort(w.f.cfg.TLS.Mode))
		}
		return huh.NewInput().Title("Host port").Value(&w.port).Validate(validPort)
	}},
	{flag: "bind-ip", field: func(w *wizard) huh.Field {
		return huh.NewSelect[string]().Title("Listen on").
			Options(
				huh.NewOption("127.0.0.1: this machine only (recommended)", "127.0.0.1"),
				huh.NewOption("0.0.0.0: all network interfaces", "0.0.0.0"),
			).Value(&w.f.cfg.BindIP)
	}},
	{flag: "expose", field: func(w *wizard) huh.Field {
		return huh.NewMultiSelect[string]().Title("Extra ports to publish").
			Description("Space to select, Enter to confirm. Usually none.").
			Options(
				huh.NewOption("mongodb: 27017, no authentication", workspace.ExposeMongoDB),
				huh.NewOption("opensearch: 9200, security plugin off", workspace.ExposeOpenSearch),
				huh.NewOption("debug: 5005, Java debugger", workspace.ExposeDebug),
			).Value(&w.f.cfg.Expose)
	}},
	{flag: "with", field: func(w *wizard) huh.Field {
		return huh.NewMultiSelect[string]().Title("Extra services").
			Description("Space to select, Enter to confirm.").
			Options(huh.NewOption("dashboards: OpenSearch Dashboards on port 5601", workspace.WithDashboards)).
			Value(&w.f.cfg.With)
	}},
	{flag: "memory", field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("Memory").
			Description("auto uses the memory reported by Docker. Or a size such as 4g.").
			Value(&w.f.cfg.Memory).Validate(func(s string) error {
			if s == "" {
				return nil
			}
			_, err := workspace.ParseMemory(s)
			return err
		})
	}},
	{flag: "version", field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("OpenArch version").Description("Image tag of ghcr.io/hyland/openarch-app-jvm.").
			Value(&w.f.cfg.Version)
	}},
	{flag: "insecure-backends", field: func(w *wizard) huh.Field {
		return huh.NewSelect[bool]().Title("MongoDB and OpenSearch security").
			Options(
				huh.NewOption("on: generated passwords (recommended)", false),
				huh.NewOption("off: no authentication", true),
			).Value(&w.f.insecureBackends)
	}},
	{flag: "default-users", field: func(w *wizard) huh.Field {
		return huh.NewSelect[bool]().Title("OpenArch users").
			Options(
				huh.NewOption("admin only, with a new password (recommended)", false),
				huh.NewOption("users bundled in the image: admin, testuser and others, password \"password\"", true),
			).Value(&w.f.cfg.Security.DefaultUsers)
	}},
	{flag: "admin-password-stdin", skip: func(w *wizard) bool { return w.f.cfg.Security.DefaultUsers }, field: func(w *wizard) huh.Field {
		return huh.NewInput().Title("Admin password").
			Description("Leave empty to generate one. It is stored in .env.local.").
			EchoMode(huh.EchoModePassword).Value(&w.password).
			Validate(func(s string) error {
				if s != "" && len(s) < 8 {
					return errors.New("use at least 8 characters")
				}
				return nil
			})
	}},
}

func notCustomTLS(w *wizard) bool { return w.f.cfg.TLS.Mode != workspace.TLSCustom }

func fileExists(s string) error {
	if _, err := os.Stat(s); err != nil {
		return errors.New("file not found")
	}
	return nil
}

// validPort accepts an empty answer, which keeps the default port.
func validPort(s string) error {
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err != nil || n < 1 || n > 65535 {
		return errors.New("use a port between 1 and 65535")
	}
	return nil
}

// runWizard asks the questions whose flag was not given. It returns the
// password typed by the user, or "" to keep the default behavior.
func runWizard(ctx context.Context, fl *pflag.FlagSet, f *initFlags, in io.Reader, out io.Writer) (string, error) {
	w := &wizard{f: f}
	if fl.Changed("port") {
		w.port = strconv.Itoa(f.cfg.Port)
	}
	for _, q := range questions {
		if fl.Changed(q.flag) || (q.skip != nil && q.skip(w)) {
			continue
		}
		form := huh.NewForm(huh.NewGroup(q.field(w))).
			WithInput(in).WithOutput(out).
			WithAccessible(os.Getenv(envAccessible) != "")
		if err := form.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return "", output.NewError(output.ExitError, "init cancelled", "")
			}
			return "", err
		}
		w.fillDefaults()
	}
	return w.password, nil
}

// fillDefaults puts the default back into answers left empty. Accessible mode
// returns an empty string when the user just presses Enter.
func (w *wizard) fillDefaults() {
	c, def := &w.f.cfg, workspace.Default("")
	if c.Storage.Mode == workspace.StorageBind && strings.TrimSpace(c.Storage.DataDir) == "" {
		c.Storage.DataDir = workspace.DefaultDataDir
	}
	if strings.TrimSpace(c.ServerName) == "" {
		c.ServerName = def.ServerName
	}
	if strings.TrimSpace(c.Memory) == "" {
		c.Memory = def.Memory
	}
	if strings.TrimSpace(c.Version) == "" {
		c.Version = def.Version
	}
	if w.port != "" {
		c.Port, _ = strconv.Atoi(w.port)
	} else {
		c.Port = 0 // init picks the default port for the TLS mode.
	}
}

// equivalentCommand returns the non-interactive command that gives the same workspace.
func equivalentCommand(dirFlag string, f *initFlags, password bool) string {
	def := workspace.Default("")
	args := []string{"openarch", "init"}
	add := func(flag, value string) { args = append(args, "--"+flag, shellQuote(value)) }
	if dirFlag != "" {
		add("dir", dirFlag)
	}
	c := f.cfg
	if c.Version != def.Version {
		add("version", c.Version)
	}
	if c.Storage.Mode != def.Storage.Mode {
		add("storage", c.Storage.Mode)
	}
	if c.Storage.Mode == workspace.StorageBind && c.Storage.DataDir != workspace.DefaultDataDir {
		add("data-dir", c.Storage.DataDir)
	}
	if c.TLS.Mode != def.TLS.Mode {
		add("tls", c.TLS.Mode)
	}
	if c.TLS.Mode == workspace.TLSCustom {
		add("cert", c.TLS.Cert)
		add("key", c.TLS.Key)
	}
	if c.ServerName != def.ServerName {
		add("server-name", c.ServerName)
	}
	if c.Port != 0 && c.Port != workspace.DefaultPort(c.TLS.Mode) {
		add("port", strconv.Itoa(c.Port))
	}
	if c.BindIP != def.BindIP {
		add("bind-ip", c.BindIP)
	}
	for _, e := range c.Expose {
		add("expose", e)
	}
	for _, e := range c.With {
		add("with", e)
	}
	if c.Memory != def.Memory {
		add("memory", c.Memory)
	}
	if f.insecureBackends {
		args = append(args, "--insecure-backends")
	}
	if c.Security.DefaultUsers {
		args = append(args, "--default-users")
	}
	if f.force {
		args = append(args, "--force")
	}
	if password {
		args = append(args, "--admin-password-stdin")
	}
	return strings.Join(append(args, "--non-interactive"), " ")
}

// shellQuote quotes s for POSIX shells when it has special characters.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@,", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// wizardFlags returns the flags the wizard can set, in question order.
func wizardFlags() []string {
	var flags []string
	for _, q := range questions {
		flags = append(flags, q.flag)
	}
	return flags
}
