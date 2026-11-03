package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"openarch": func() { os.Exit(run()) },
	})
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("..", "..", "testdata", "script"),
		Setup: func(env *testscript.Env) error {
			// Keep the scripts away from the real CLI config and the OS credential store.
			env.Setenv("OPENARCH_CONFIG_DIR", filepath.Join(env.WorkDir, ".openarch"))
			env.Setenv("OPENARCH_CREDENTIAL_STORE", "file")
			return nil
		},
	})
}
