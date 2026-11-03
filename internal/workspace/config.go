// Package workspace loads, validates and saves openarch.yaml, the source of
// truth of a workspace, and writes the files rendered from it.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// ConfigFile is the name of the workspace config file.
const ConfigFile = "openarch.yaml"

// SchemaVersion is the version of the openarch.yaml schema written by this CLI.
const SchemaVersion = 1

// DefaultVersion is the OpenArch image tag tested with this CLI.
const DefaultVersion = "1.0.90"

// Storage modes.
const (
	StorageNone   = "none"
	StorageVolume = "volume"
	StorageBind   = "bind"
)

// TLS modes.
const (
	TLSNone       = "none"
	TLSSelfSigned = "self-signed"
	TLSCustom     = "custom"
)

// Values for Expose and With.
const (
	ExposeMongoDB    = "mongodb"
	ExposeOpenSearch = "opensearch"
	ExposeDebug      = "debug"
	WithDashboards   = "dashboards"
)

var (
	StorageModes  = []string{StorageNone, StorageVolume, StorageBind}
	TLSModes      = []string{TLSNone, TLSSelfSigned, TLSCustom}
	ExposeOptions = []string{ExposeMongoDB, ExposeOpenSearch, ExposeDebug}
	WithOptions   = []string{WithDashboards}
)

// Config is the content of openarch.yaml. JSON keys match the YAML keys.
type Config struct {
	SchemaVersion int      `yaml:"schemaVersion" json:"schemaVersion"`
	Name          string   `yaml:"name" json:"name"`
	Version       string   `yaml:"version" json:"version"`
	Storage       Storage  `yaml:"storage" json:"storage"`
	TLS           TLS      `yaml:"tls" json:"tls"`
	ServerName    string   `yaml:"serverName" json:"serverName"`
	Port          int      `yaml:"port" json:"port"`
	BindIP        string   `yaml:"bindIP" json:"bindIP"`
	Expose        []string `yaml:"expose,omitempty" json:"expose,omitempty"`
	With          []string `yaml:"with,omitempty" json:"with,omitempty"`
	Memory        string   `yaml:"memory" json:"memory"`
	// InsecureBackends turns off authentication in MongoDB and the OpenSearch security plugin.
	InsecureBackends bool `yaml:"insecureBackends,omitempty" json:"insecureBackends,omitempty"`
	// DefaultUsers keeps the users bundled in the OpenArch image (password "password").
	DefaultUsers bool `yaml:"defaultUsers,omitempty" json:"defaultUsers,omitempty"`
}

// Storage selects where data is kept.
type Storage struct {
	Mode    string `yaml:"mode" json:"mode"`
	DataDir string `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`
}

// TLS selects how HTTPS is served.
type TLS struct {
	Mode string `yaml:"mode" json:"mode"`
	Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
	Key  string `yaml:"key,omitempty" json:"key,omitempty"`
}

// DefaultPort returns the default host port for a TLS mode.
func DefaultPort(tlsMode string) int {
	if tlsMode == "" || tlsMode == TLSNone {
		return 8080
	}
	return 8443
}

// DefaultDataDir is the data folder used with storage bind.
const DefaultDataDir = "./data"

// Default returns the default config for a workspace called name.
func Default(name string) Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Name:          ProjectName(name),
		Version:       DefaultVersion,
		Storage:       Storage{Mode: StorageVolume},
		TLS:           TLS{Mode: TLSNone},
		ServerName:    "localhost",
		Port:          DefaultPort(TLSNone),
		BindIP:        "127.0.0.1",
		Memory:        "auto",
	}
}

var nonProjectChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// ProjectName turns s into a valid Docker Compose project name.
func ProjectName(s string) string {
	n := nonProjectChars.ReplaceAllString(strings.ToLower(s), "-")
	n = strings.Trim(n, "-_")
	if n == "" {
		return "openarch"
	}
	return n
}

var (
	projectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	imageTag    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	hostName    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)
	memorySize  = regexp.MustCompile(`^(?i)([0-9]+)(m|g)b?$`)
)

// Validate checks every field and returns a usage error for the first problem found.
func (c Config) Validate() error {
	bad := func(field, msg, hint string) error {
		return output.Usage(fmt.Sprintf("invalid %s: %s", field, msg), hint)
	}
	if c.SchemaVersion != SchemaVersion {
		return bad("schemaVersion", strconv.Itoa(c.SchemaVersion), fmt.Sprintf("This CLI supports schemaVersion %d.", SchemaVersion))
	}
	if !projectName.MatchString(c.Name) {
		return bad("name", strconv.Quote(c.Name), "Use lowercase letters, digits, '-' and '_'.")
	}
	if !imageTag.MatchString(c.Version) {
		return bad("version", strconv.Quote(c.Version), "Use an image tag such as "+DefaultVersion+".")
	}
	if !slices.Contains(StorageModes, c.Storage.Mode) {
		return bad("storage", strconv.Quote(c.Storage.Mode), "Valid values: "+strings.Join(StorageModes, ", ")+".")
	}
	if c.Storage.Mode == StorageBind && c.Storage.DataDir == "" {
		return bad("dataDir", "empty", "Set a data folder for storage bind.")
	}
	if c.Storage.Mode != StorageBind && c.Storage.DataDir != "" {
		return bad("dataDir", "only valid with storage bind", "Remove the data folder or use storage bind.")
	}
	if !slices.Contains(TLSModes, c.TLS.Mode) {
		return bad("tls", strconv.Quote(c.TLS.Mode), "Valid values: "+strings.Join(TLSModes, ", ")+".")
	}
	if c.TLS.Mode == TLSCustom && (c.TLS.Cert == "" || c.TLS.Key == "") {
		return bad("tls", "custom needs a certificate and a key", "Set both --cert and --key.")
	}
	if c.TLS.Mode != TLSCustom && (c.TLS.Cert != "" || c.TLS.Key != "") {
		return bad("tls", "certificate files are only valid with tls custom", "Remove --cert and --key, or use --tls custom.")
	}
	if len(c.ServerName) > 253 || !hostName.MatchString(c.ServerName) {
		return bad("serverName", strconv.Quote(c.ServerName), "Use a host name such as localhost or openarch.example.com.")
	}
	if c.Port < 1 || c.Port > 65535 {
		return bad("port", strconv.Itoa(c.Port), "Use a port between 1 and 65535.")
	}
	if ip := net.ParseIP(c.BindIP); ip == nil {
		return bad("bindIP", strconv.Quote(c.BindIP), "Use 127.0.0.1 (this machine only) or 0.0.0.0 (all interfaces).")
	}
	if err := checkList("expose", c.Expose, ExposeOptions); err != nil {
		return err
	}
	if err := checkList("with", c.With, WithOptions); err != nil {
		return err
	}
	if _, err := ParseMemory(c.Memory); err != nil {
		return err
	}
	for _, p := range c.HostPorts() {
		if p.Port == c.Port && p.Name != "openarch" {
			return bad("port", fmt.Sprintf("%d is also used by %s", c.Port, p.Name), "Choose another port.")
		}
	}
	return nil
}

func checkList(field string, values, valid []string) error {
	seen := map[string]bool{}
	for _, v := range values {
		if !slices.Contains(valid, v) {
			return output.Usage(fmt.Sprintf("invalid %s: %q", field, v), "Valid values: "+strings.Join(valid, ", ")+".")
		}
		if seen[v] {
			return output.Usage(fmt.Sprintf("invalid %s: %q is listed twice", field, v), "")
		}
		seen[v] = true
	}
	return nil
}

// ParseMemory parses "auto" or a size such as 4g or 4096m. It returns 0 for auto.
func ParseMemory(s string) (int64, error) {
	if s == "auto" {
		return 0, nil
	}
	m := memorySize.FindStringSubmatch(s)
	if m == nil {
		return 0, output.Usage(fmt.Sprintf("invalid memory: %q", s), "Use auto or a size such as 4g or 4096m.")
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n == 0 || n > 1<<20 {
		return 0, output.Usage(fmt.Sprintf("invalid memory: %q", s), "Use auto or a size such as 4g or 4096m.")
	}
	if strings.EqualFold(m[2], "g") {
		n *= 1024
	}
	return n << 20, nil
}

// TLSEnabled reports whether HTTPS is served by the proxy.
func (c Config) TLSEnabled() bool { return c.TLS.Mode != TLSNone }

// Has reports whether v is in Expose or With.
func (c Config) Has(v string) bool { return slices.Contains(c.Expose, v) || slices.Contains(c.With, v) }

// HostPort is a port published on the host.
type HostPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// HostPorts returns every port the workspace publishes on the host, main port first.
func (c Config) HostPorts() []HostPort {
	ports := []HostPort{{Name: "openarch", Port: c.Port}}
	if c.Has(ExposeMongoDB) {
		ports = append(ports, HostPort{Name: ExposeMongoDB, Port: 27017})
	}
	if c.Has(ExposeOpenSearch) {
		ports = append(ports, HostPort{Name: ExposeOpenSearch, Port: 9200})
	}
	if c.Has(ExposeDebug) {
		ports = append(ports, HostPort{Name: ExposeDebug, Port: 5005})
	}
	if c.Has(WithDashboards) {
		ports = append(ports, HostPort{Name: WithDashboards, Port: 5601})
	}
	return ports
}

// URL returns the OpenArch URL as seen from the host.
func (c Config) URL() string {
	scheme, def := "http", 80
	if c.TLSEnabled() {
		scheme, def = "https", 443
	}
	if c.Port == def {
		return scheme + "://" + c.ServerName
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.ServerName, c.Port)
}

// Warnings returns the risky choices in the config. They do not make it invalid.
func (c Config) Warnings() []string {
	var w []string
	if ip := net.ParseIP(c.BindIP); ip != nil && !ip.IsLoopback() && !c.TLSEnabled() {
		w = append(w, "OpenArch listens on "+c.BindIP+" without TLS. Passwords are sent in clear text (HTTP Basic auth). Use --tls self-signed or --tls custom.")
	}
	if c.DefaultUsers {
		w = append(w, "OpenArch uses the users bundled in its image (admin, testuser and others), all with the password \"password\".")
	}
	if c.InsecureBackends {
		w = append(w, "MongoDB and OpenSearch run with no authentication. Any container on the Docker network can read and change the data.")
		if c.Has(ExposeOpenSearch) {
			w = append(w, "OpenSearch is published on port 9200 with no authentication. Anyone who can reach the port can read and change the index.")
		}
		if c.Has(ExposeMongoDB) {
			w = append(w, "MongoDB is published on port 27017 with no authentication.")
		}
		if c.Has(WithDashboards) {
			w = append(w, "OpenSearch Dashboards is published on port 5601 with no authentication.")
		}
	}
	if !c.InsecureBackends && c.Has(ExposeOpenSearch) {
		w = append(w, "OpenSearch is published on port 9200 over plain HTTP. Its passwords are sent in clear text on that port.")
	}
	if c.Storage.Mode == StorageNone {
		w = append(w, "Storage none keeps no data: everything is lost when the containers are removed.")
	}
	return w
}

// Load reads and validates openarch.yaml from path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, output.Usage(fmt.Sprintf("%s not found", path), "Run 'openarch init' to create a workspace, or use --dir.")
	}
	if err != nil {
		return Config{}, err
	}
	return Parse(data, path)
}

// Parse decodes and validates openarch.yaml content. name is used in error messages.
func Parse(data []byte, name string) (Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, output.Usage(fmt.Sprintf("cannot parse %s: %v", name, err), "Fix the file or create it again with 'openarch init --force'.")
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Marshal encodes c as openarch.yaml content.
func (c Config) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString("# OpenArch workspace. This file is the source of truth.\n")
	b.WriteString("# Edit it, then run 'openarch render' to update the other files.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		// Config holds only strings, ints and slices, so encoding cannot fail.
		panic(err)
	}
	return b.Bytes()
}

// Find returns the workspace directory: dir if set, then OPENARCH_WORKSPACE, then the current directory.
func Find(dir string) (string, error) {
	if dir == "" {
		dir = os.Getenv("OPENARCH_WORKSPACE")
	}
	if dir == "" {
		dir = "."
	}
	return filepath.Abs(dir)
}
