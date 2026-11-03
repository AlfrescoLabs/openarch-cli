package cli

import (
	"os"
	"testing"
)

// TestMain keeps the tests away from the real CLI config and the OS credential store.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "openarch-cli-test")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("OPENARCH_CONFIG_DIR", dir)
	_ = os.Setenv("OPENARCH_CREDENTIAL_STORE", "file")
	_ = os.Unsetenv("OPENARCH_CONTEXT")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
