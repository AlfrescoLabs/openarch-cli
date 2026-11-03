// Package contexts reads and writes the CLI config file with the named
// connections ("contexts"). The file never holds a password.
package contexts

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// EnvConfigDir overrides the config folder. EnvContext selects a context.
const (
	EnvConfigDir = "OPENARCH_CONFIG_DIR"
	EnvContext   = "OPENARCH_CONTEXT"
)

// SystemOpenArch is the endpoint system of an OpenArch server.
const SystemOpenArch = "openarch"

// Systems lists the valid endpoint systems.
var Systems = []string{SystemOpenArch}

// Endpoint is the connection to one system. The password is in the credential store.
type Endpoint struct {
	URL      string `yaml:"url" json:"url"`
	Username string `yaml:"username" json:"username"`
	Insecure bool   `yaml:"insecure,omitempty" json:"insecure,omitempty"`
}

// Context is a named connection: a local workspace, or remote endpoints.
type Context struct {
	Workspace string              `yaml:"workspace,omitempty" json:"workspace,omitempty"`
	Endpoints map[string]Endpoint `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
}

// Kind is "workspace" or "remote".
func (c Context) Kind() string {
	if c.Workspace != "" {
		return "workspace"
	}
	return "remote"
}

// Config is the content of config.yaml.
type Config struct {
	CurrentContext string             `yaml:"current-context,omitempty"`
	Contexts       map[string]Context `yaml:"contexts,omitempty"`
}

// Dir returns the config folder: OPENARCH_CONFIG_DIR, then XDG_CONFIG_HOME/openarch,
// then %AppData%\openarch on Windows and ~/.config/openarch elsewhere.
func Dir(getenv func(string) string) (string, error) {
	if d := getenv(EnvConfigDir); d != "" {
		return d, nil
	}
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "openarch"), nil
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "openarch"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "openarch"), nil
}

// File is the config file in a config folder.
func File(dir string) string { return filepath.Join(dir, "config.yaml") }

// Load reads the config file. A missing file is an empty config.
func Load(dir string) (Config, error) {
	var c Config
	data, err := os.ReadFile(File(dir))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, output.Usage(fmt.Sprintf("cannot parse %s: %v", File(dir), err), "Fix the file, or remove it to start again.")
	}
	return c, nil
}

// Save writes the config file.
func Save(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString("# openarch CLI connections. Passwords are not in this file:\n# they are in the OS credential store, or in credentials.json next to it.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return err
	}
	tmp := File(dir) + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, File(dir))
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// ValidateName checks a context name.
func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return output.Usage(fmt.Sprintf("invalid context name %q", name), "Use letters, digits, '.', '_' and '-'.")
	}
	return nil
}

// Names returns the context names, sorted.
func (c Config) Names() []string {
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get returns a context, or a usage error that lists the valid names.
func (c Config) Get(name string) (Context, error) {
	ctx, ok := c.Contexts[name]
	if !ok {
		hint := "Run 'openarch config list'."
		if len(c.Contexts) == 0 {
			hint = "There are no contexts yet. Create one with 'openarch config set'."
		}
		return Context{}, output.Usage(fmt.Sprintf("unknown context %q", name), hint)
	}
	return ctx, nil
}

// NameForWorkspace returns the name of the context that points to dir, or "".
func (c Config) NameForWorkspace(dir string) string {
	for _, n := range c.Names() {
		if c.Contexts[n].Workspace == dir {
			return n
		}
	}
	return ""
}

// ValidSystem checks an endpoint system name.
func ValidSystem(s string) error {
	if !slices.Contains(Systems, s) {
		return output.Usage(fmt.Sprintf("unknown system %q", s), "Valid systems: openarch.")
	}
	return nil
}
