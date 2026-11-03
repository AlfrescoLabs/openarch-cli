// Package credstore keeps passwords in the OS credential store, or in a file
// when no OS store is available.
package credstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service is the label of the entries in the OS store.
const Service = "openarch"

// EnvStore forces a backend: keychain or file.
const EnvStore = "OPENARCH_CREDENTIAL_STORE"

// ErrNotFound means the store has no password for the URL and user.
var ErrNotFound = errors.New("no stored password")

// Store keeps one password per URL and user.
type Store interface {
	Get(url, user string) (string, error)
	Set(url, user, secret string) error
	Delete(url, user string) error
	// Name is "keychain", "file" or "memory".
	Name() string
}

func account(url, user string) string { return user + "@" + url }

// Keychain uses the OS store: Keychain on macOS, Credential Manager on
// Windows, Secret Service over D-Bus on Linux.
type Keychain struct{}

func (Keychain) Name() string { return "keychain" }

func (Keychain) Get(url, user string) (string, error) {
	s, err := keyring.Get(Service, account(url, user))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return s, err
}

func (Keychain) Set(url, user, secret string) error {
	return keyring.Set(Service, account(url, user), secret)
}

func (Keychain) Delete(url, user string) error {
	err := keyring.Delete(Service, account(url, user))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// File keeps passwords in a JSON file with mode 0600.
type File struct {
	Path string
	mu   sync.Mutex
}

type fileEntry struct {
	URL      string `json:"url"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type fileData struct {
	Version     int         `json:"version"`
	Credentials []fileEntry `json:"credentials"`
}

func (f *File) Name() string { return "file" }

func (f *File) load() (fileData, error) {
	d := fileData{Version: 1}
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, fmt.Errorf("parse %s: %w", f.Path, err)
	}
	return d, nil
}

func (f *File) save(d fileData) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}

func (f *File) Get(url, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, err := f.load()
	if err != nil {
		return "", err
	}
	for _, e := range d.Credentials {
		if e.URL == url && e.User == user {
			return e.Password, nil
		}
	}
	return "", ErrNotFound
}

func (f *File) Set(url, user, secret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, err := f.load()
	if err != nil {
		return err
	}
	d.Credentials = slices.DeleteFunc(d.Credentials, func(e fileEntry) bool { return e.URL == url && e.User == user })
	d.Credentials = append(d.Credentials, fileEntry{URL: url, User: user, Password: secret})
	return f.save(d)
}

func (f *File) Delete(url, user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, err := f.load()
	if err != nil {
		return err
	}
	n := len(d.Credentials)
	d.Credentials = slices.DeleteFunc(d.Credentials, func(e fileEntry) bool { return e.URL == url && e.User == user })
	if len(d.Credentials) == n {
		return nil
	}
	return f.save(d)
}

// Memory is a store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *Memory) Name() string { return "memory" }

func (m *Memory) Get(url, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[account(url, user)]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (m *Memory) Set(url, user, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[account(url, user)] = secret
	return nil
}

func (m *Memory) Delete(url, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, account(url, user))
	return nil
}

// Open returns the store to use. It is the OS store when it works, else the
// file store. warn is called when the file store replaces the OS store.
// OPENARCH_CREDENTIAL_STORE=keychain|file forces a backend.
func Open(configDir string, getenv func(string) string, warn func(string)) (Store, error) {
	file := &File{Path: filepath.Join(configDir, "credentials.json")}
	switch getenv(EnvStore) {
	case "file":
		return file, nil
	case "keychain":
		return Keychain{}, nil
	case "":
	default:
		return nil, fmt.Errorf("invalid %s: %q (use keychain or file)", EnvStore, getenv(EnvStore))
	}
	// A lookup of an entry that does not exist tells whether the OS store works.
	if _, err := (Keychain{}).Get("probe", "openarch"); err == nil || errors.Is(err, ErrNotFound) {
		return Keychain{}, nil
	} else if warn != nil {
		warn(fmt.Sprintf("The OS credential store is not available (%v). Passwords are stored in %s (mode 0600). Set %s=file to hide this warning.", err, file.Path, EnvStore))
	}
	return file, nil
}
