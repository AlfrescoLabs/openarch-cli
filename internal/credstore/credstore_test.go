package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func exercise(t *testing.T, s Store) {
	t.Helper()
	if _, err := s.Get("https://a", "u"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on empty store = %v", err)
	}
	if err := s.Set("https://a", "u", "p1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("https://a", "u", "p2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("https://b", "u", "p3"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("https://a", "u"); err != nil || got != "p2" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if err := s.Delete("https://a", "u"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("https://a", "u"); err != nil {
		t.Errorf("second Delete = %v", err)
	}
	if _, err := s.Get("https://a", "u"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v", err)
	}
	if got, _ := s.Get("https://b", "u"); got != "p3" {
		t.Errorf("other entry = %q", got)
	}
}

func TestMemory(t *testing.T) { exercise(t, &Memory{}) }

func TestFile(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "sub", "credentials.json")}
	exercise(t, f)
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func TestOpenForcedBackend(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	dir := t.TempDir()
	if s, err := Open(dir, env("file"), nil); err != nil || s.Name() != "file" {
		t.Errorf("file: %v, %v", s, err)
	}
	if s, err := Open(dir, env("keychain"), nil); err != nil || s.Name() != "keychain" {
		t.Errorf("keychain: %v, %v", s, err)
	}
	if _, err := Open(dir, env("vault"), nil); err == nil {
		t.Error("unknown backend accepted")
	}
}
