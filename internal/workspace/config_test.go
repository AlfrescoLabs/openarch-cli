package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func TestDefaultIsValid(t *testing.T) {
	c := Default("My OpenArch!")
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Name != "my-openarch" {
		t.Errorf("Name = %q", c.Name)
	}
	if c.URL() != "http://localhost:8080" {
		t.Errorf("URL = %q", c.URL())
	}
	if len(c.Warnings()) != 0 {
		t.Errorf("Warnings = %v", c.Warnings())
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"schema", func(c *Config) { c.SchemaVersion = 2 }, "invalid schemaVersion"},
		{"name", func(c *Config) { c.Name = "Bad Name" }, "invalid name"},
		{"version", func(c *Config) { c.Version = "1.0:x" }, "invalid version"},
		{"storage", func(c *Config) { c.Storage.Mode = "disk" }, "invalid storage"},
		{"bind without dir", func(c *Config) { c.Storage.Mode = StorageBind }, "invalid dataDir"},
		{"dir without bind", func(c *Config) { c.Storage.DataDir = "./data" }, "only valid with storage bind"},
		{"tls", func(c *Config) { c.TLS.Mode = "acme" }, "invalid tls"},
		{"custom without files", func(c *Config) { c.TLS.Mode = TLSCustom; c.TLS.Cert = "c.pem" }, "needs a certificate and a key"},
		{"files without custom", func(c *Config) { c.TLS.Cert = "c.pem" }, "only valid with tls custom"},
		{"server name", func(c *Config) { c.ServerName = "bad_name" }, "invalid serverName"},
		{"port", func(c *Config) { c.Port = 70000 }, "invalid port"},
		{"bind ip", func(c *Config) { c.BindIP = "localhost" }, "invalid bindIP"},
		{"expose", func(c *Config) { c.Expose = []string{"redis"} }, "invalid expose"},
		{"expose twice", func(c *Config) { c.Expose = []string{ExposeDebug, ExposeDebug} }, "listed twice"},
		{"with", func(c *Config) { c.With = []string{"kibana"} }, "invalid with"},
		{"memory", func(c *Config) { c.Memory = "lots" }, "invalid memory"},
		{"port clash", func(c *Config) { c.Port = 9200; c.Expose = []string{ExposeOpenSearch} }, "also used by opensearch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default("x")
			tt.change(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want %q", err, tt.want)
			}
			if output.CodeOf(err) != output.ExitUsage {
				t.Errorf("exit code = %d, want usage", output.CodeOf(err))
			}
		})
	}
}

func TestParseMemory(t *testing.T) {
	for in, want := range map[string]int64{"auto": 0, "4g": 4 << 30, "4G": 4 << 30, "512m": 512 << 20, "2gb": 2 << 30} {
		got, err := ParseMemory(in)
		if err != nil || got != want {
			t.Errorf("ParseMemory(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "4", "0g", "4t", "-1g", "4 g"} {
		if _, err := ParseMemory(in); err == nil {
			t.Errorf("ParseMemory(%q) succeeded", in)
		}
	}
}

func TestSizingFor(t *testing.T) {
	for budget, heap := range map[int64]string{4 << 30: "1g", 8 << 30: "2g", 16 << 30: "4g"} {
		if got := SizingFor(budget).OpenArchHeap; got != heap {
			t.Errorf("SizingFor(%d).OpenArchHeap = %s, want %s", budget, got, heap)
		}
	}
}

func TestURLAndPorts(t *testing.T) {
	c := Default("x")
	c.TLS.Mode, c.Port, c.ServerName = TLSSelfSigned, 443, "oa.example.com"
	c.Expose = []string{ExposeDebug}
	c.With = []string{WithDashboards}
	if c.URL() != "https://oa.example.com" {
		t.Errorf("URL = %q", c.URL())
	}
	var ports []int
	for _, p := range c.HostPorts() {
		ports = append(ports, p.Port)
	}
	if len(ports) != 3 || ports[0] != 443 || ports[1] != 5005 || ports[2] != 5601 {
		t.Errorf("HostPorts = %v", ports)
	}
}

func TestWarnings(t *testing.T) {
	c := Default("x")
	c.BindIP = "0.0.0.0"
	c.Expose = []string{ExposeOpenSearch}
	w := strings.Join(c.Warnings(), "\n")
	if !strings.Contains(w, "without TLS") || !strings.Contains(w, "security plugin") {
		t.Errorf("Warnings = %q", w)
	}
	c.TLS.Mode = TLSSelfSigned
	if strings.Contains(strings.Join(c.Warnings(), "\n"), "without TLS") {
		t.Error("TLS warning with TLS on")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	c := Default("x")
	c.Storage = Storage{Mode: StorageBind, DataDir: "./data"}
	c.Expose = []string{ExposeDebug}
	path := filepath.Join(t.TempDir(), ConfigFile)
	if err := os.WriteFile(path, c.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Marshal()) != string(c.Marshal()) {
		t.Errorf("round trip changed the config:\n%s", got.Marshal())
	}
}

func TestParseRejectsUnknownKeys(t *testing.T) {
	data := append(Default("x").Marshal(), []byte("colour: blue\n")...)
	if _, err := Parse(data, "openarch.yaml"); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Errorf("Parse = %v", err)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), ConfigFile))
	if output.CodeOf(err) != output.ExitUsage {
		t.Errorf("Load = %v", err)
	}
}
