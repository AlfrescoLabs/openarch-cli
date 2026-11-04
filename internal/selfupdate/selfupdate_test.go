package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func TestVerifyEntity(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	v, err := verify.NewVerifier(vs, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	if err != nil {
		t.Fatal(err)
	}
	sums := []byte("abc  openarch_0.2.0_linux_amd64.tar.gz\n")
	cases := []struct {
		name     string
		identity string
		issuer   string
		data     []byte
		ok       bool
	}{
		{"release workflow at the tag", Identity("v0.2.0"), Issuer, sums, true},
		{"another tag", Identity("v0.1.9"), Issuer, sums, false},
		{"another workflow", "https://github.com/" + Repo + "/.github/workflows/ci.yml@refs/heads/main", Issuer, sums, false},
		{"another repository", "https://github.com/someone/openarch-cli/.github/workflows/release.yml@refs/tags/v0.2.0", Issuer, sums, false},
		{"another issuer", Identity("v0.2.0"), "https://accounts.google.com", sums, false},
		{"other content", Identity("v0.2.0"), Issuer, []byte("changed"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := vs.Sign(c.identity, c.issuer, c.data)
			if err != nil {
				t.Fatal(err)
			}
			err = verifyEntity(v, "v0.2.0", sums, e)
			if (err == nil) != c.ok {
				t.Fatalf("ok = %v, want %v (err %v)", err == nil, c.ok, err)
			}
		})
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"0.2.0": true, "v0.2.0": true, "dev": false, "v0.1.0-3-g3254804": false,
		"v0.1.0-3-g3254804-dirty": false, "0.2.1-next": false, "": false,
	} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("v0.2.0", "darwin", "arm64"); got != "openarch_0.2.0_darwin_arm64.tar.gz" {
		t.Errorf("got %s", got)
	}
	if got := AssetName("0.2.0", "windows", "amd64"); got != "openarch_0.2.0_windows_amd64.zip" {
		t.Errorf("got %s", got)
	}
}

func TestPackageManager(t *testing.T) {
	for exe, want := range map[string]string{
		"/opt/homebrew/Cellar/openarch/0.2.0/bin/openarch":     "brew",
		"/home/linuxbrew/.linuxbrew/bin/openarch":              "brew",
		`C:\Users\me\scoop\apps\openarch\current\openarch.exe`: "scoop",
		"/usr/local/bin/openarch":                              "",
	} {
		if got := PackageManager(exe); got != want {
			t.Errorf("PackageManager(%q) = %q, want %q", exe, got, want)
		}
	}
}

// release is a fake GitHub release server.
type release struct {
	tag     string
	files   map[string][]byte
	fetched []string
}

func (r *release) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/"+Repo+"/releases/latest" {
			w.Header().Set("Location", "https://github.com/"+Repo+"/releases/tag/"+r.tag)
			w.WriteHeader(http.StatusFound)
			return
		}
		prefix := "/" + Repo + "/releases/download/" + r.tag + "/"
		name := strings.TrimPrefix(req.URL.Path, prefix)
		data, ok := r.files[name]
		if !strings.HasPrefix(req.URL.Path, prefix) || !ok {
			http.NotFound(w, req)
			return
		}
		r.fetched = append(r.fetched, name)
		_, _ = w.Write(data)
	})
}

func tarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		data []byte
	}{{"README.md", []byte("readme")}, {name, data}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipFile(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newRelease(t *testing.T, goos string, archive []byte) (*release, Env) {
	t.Helper()
	asset := AssetName("v0.2.0", goos, "amd64")
	sum := sha256.Sum256(archive)
	r := &release{tag: "v0.2.0", files: map[string][]byte{
		asset:         archive,
		ChecksumsFile: []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)),
		BundleFile:    []byte("{}"),
	}}
	srv := httptest.NewServer(r.handler())
	t.Cleanup(srv.Close)
	env := Env{
		HTTP:       srv.Client(),
		BaseURL:    srv.URL,
		Executable: func() (string, error) { return "/usr/local/bin/openarch", nil },
		GOOS:       goos,
		GOARCH:     "amd64",
		Verify:     func(context.Context, string, []byte, []byte) error { return nil },
	}
	return r, env
}

func TestPrepareLatest(t *testing.T) {
	_, env := newRelease(t, "linux", nil)
	p, err := Prepare(context.Background(), env, "0.1.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != "0.2.0" || !p.UpdateAvailable || p.Asset != "openarch_0.2.0_linux_amd64.tar.gz" {
		t.Errorf("plan = %+v", p)
	}
	if p, _ := Prepare(context.Background(), env, "0.2.0", ""); p.UpdateAvailable {
		t.Error("same version: update available")
	}
	if p, _ := Prepare(context.Background(), env, "v0.1.0-3-gabc", ""); p.UpdateAvailable {
		t.Error("build from source: update available")
	}
	if _, err := Prepare(context.Background(), env, "0.1.0", "latest"); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("bad version: err %v", err)
	}
}

func TestDownload(t *testing.T) {
	bin := []byte("new binary")
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			archive := tarGz(t, "openarch", bin)
			if goos == "windows" {
				archive = zipFile(t, "openarch.exe", bin)
			}
			_, env := newRelease(t, goos, archive)
			p, err := Prepare(context.Background(), env, "0.1.0", "0.2.0")
			if err != nil {
				t.Fatal(err)
			}
			got, err := Download(context.Background(), env, p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bin) {
				t.Errorf("binary = %q", got)
			}
		})
	}
}

func TestDownloadBadSignature(t *testing.T) {
	r, env := newRelease(t, "linux", tarGz(t, "openarch", []byte("x")))
	env.Verify = func(context.Context, string, []byte, []byte) error { return signatureError(errors.New("bad")) }
	p, _ := Prepare(context.Background(), env, "0.1.0", "0.2.0")
	if _, err := Download(context.Background(), env, p); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("err = %v", err)
	}
	for _, f := range r.fetched {
		if f == p.Asset {
			t.Error("the archive was downloaded before the signature check")
		}
	}
}

func TestDownloadBadChecksum(t *testing.T) {
	r, env := newRelease(t, "linux", tarGz(t, "openarch", []byte("x")))
	p, _ := Prepare(context.Background(), env, "0.1.0", "0.2.0")
	r.files[p.Asset] = tarGz(t, "openarch", []byte("changed"))
	if _, err := Download(context.Background(), env, p); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadNoBundle(t *testing.T) {
	r, env := newRelease(t, "linux", tarGz(t, "openarch", []byte("x")))
	delete(r.files, BundleFile)
	p, _ := Prepare(context.Background(), env, "0.1.0", "0.2.0")
	_, err := Download(context.Background(), env, p)
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.ExitRemote || !strings.Contains(oe.Hint, "0.2.0") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstall(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "openarch")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := Install(exe, []byte("new"), goos); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(exe)
			if string(got) != "new" {
				t.Errorf("exe = %q", got)
			}
			if fi, _ := os.Stat(exe); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 {
				t.Errorf("mode = %v", fi.Mode())
			}
			_, err := os.Stat(exe + ".old")
			if (err == nil) != (goos == "windows") {
				t.Errorf("exe.old exists = %v", err == nil)
			}
			RemoveOld(exe)
			entries, _ := os.ReadDir(filepath.Dir(exe))
			if len(entries) != 1 {
				t.Errorf("files left: %v", entries)
			}
		})
	}
}
