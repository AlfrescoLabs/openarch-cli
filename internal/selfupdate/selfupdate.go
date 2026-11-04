// Package selfupdate replaces the running binary with a release from GitHub,
// after it checks the keyless Sigstore signature of checksums.txt and the
// SHA-256 of the archive.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

const (
	// Repo is the GitHub repository of the releases.
	Repo = "AlfrescoLabs/openarch-cli"
	// Issuer is the OIDC issuer of the release signature: GitHub Actions.
	Issuer = "https://token.actions.githubusercontent.com"
	// ChecksumsFile and BundleFile are release assets.
	ChecksumsFile = "checksums.txt"
	BundleFile    = "checksums.txt.sigstore.json"

	// maxDownload limits every download. A release archive is about 7 MB.
	maxDownload = 200 << 20
)

// Identity is the certificate identity of a release signature: the release
// workflow of Repo, running for tag.
func Identity(tag string) string {
	return "https://github.com/" + Repo + "/.github/workflows/release.yml@refs/tags/" + tag
}

// Env gives the package access to the system. Tests replace its fields.
type Env struct {
	HTTP *http.Client
	// BaseURL is https://github.com, or a test server.
	BaseURL    string
	Executable func() (string, error)
	GOOS       string
	GOARCH     string
	// Verify checks that bundleJSON is a valid signature of sums, made by the
	// release workflow for tag.
	Verify func(ctx context.Context, tag string, sums, bundleJSON []byte) error
}

// SystemEnv uses GitHub, the running executable and the public Sigstore instance.
func SystemEnv() Env {
	return Env{
		HTTP:       &http.Client{Timeout: 5 * time.Minute},
		BaseURL:    "https://github.com",
		Executable: executable,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Verify:     VerifySigstore,
	}
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// publicVerifier trusts the public Sigstore instance. Its trusted root comes
// through TUF and is cached in the user cache folder.
func publicVerifier(ctx context.Context) (*verify.Verifier, error) {
	opts := tuf.DefaultOptions().WithContext(ctx)
	if dir, err := os.UserCacheDir(); err == nil {
		opts = opts.WithCachePath(filepath.Join(dir, "openarch", "sigstore"))
	}
	trusted, err := root.NewLiveTrustedRoot(opts)
	if err != nil {
		return nil, output.NewError(output.ExitRemote, "cannot load the Sigstore trusted root: "+err.Error(),
			"Check the network access to tuf-repo-cdn.sigstore.dev.")
	}
	return verify.NewVerifier(trusted,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1))
}

// Plan describes an update.
type Plan struct {
	Current         string `json:"current"`
	Target          string `json:"target"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Asset           string `json:"asset"`
	Executable      string `json:"executable"`
}

// Prepare compares the current version with tag. An empty tag means the latest release.
func Prepare(ctx context.Context, env Env, current, tag string) (Plan, error) {
	if tag == "" {
		latest, err := LatestTag(ctx, env)
		if err != nil {
			return Plan{}, err
		}
		tag = latest
	}
	tag = "v" + strings.TrimPrefix(tag, "v")
	if !semver.IsValid(tag) {
		return Plan{}, output.Usage(fmt.Sprintf("invalid version %q", tag), "Use a release version such as 0.2.0.")
	}
	exe, err := env.Executable()
	if err != nil {
		return Plan{}, fmt.Errorf("cannot find the running executable: %w", err)
	}
	cur := "v" + strings.TrimPrefix(current, "v")
	return Plan{
		Current:         current,
		Target:          strings.TrimPrefix(tag, "v"),
		UpdateAvailable: IsRelease(current) && semver.Compare(tag, cur) > 0,
		Asset:           AssetName(tag, env.GOOS, env.GOARCH),
		Executable:      exe,
	}, nil
}

// IsRelease reports whether v is a release version such as 0.2.0. Builds from
// source (dev, or git describe output such as v0.1.0-3-gabc1234) are not.
func IsRelease(v string) bool {
	v = "v" + strings.TrimPrefix(v, "v")
	return semver.IsValid(v) && semver.Prerelease(v) == "" && semver.Build(v) == ""
}

// AssetName is the archive name of tag for one platform, as GoReleaser writes it.
func AssetName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("openarch_%s_%s_%s%s", strings.TrimPrefix(tag, "v"), goos, goarch, ext)
}

// PackageManager returns "brew" or "scoop" when exe was installed by one of them.
func PackageManager(exe string) string {
	p := strings.ReplaceAll(exe, `\`, "/")
	switch {
	case strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/"):
		return "brew"
	case strings.Contains(strings.ToLower(p), "/scoop/"):
		return "scoop"
	}
	return ""
}

// LatestTag returns the tag of the latest published release. It reads the
// redirect of /releases/latest, so it needs no GitHub API token.
func LatestTag(ctx context.Context, env Env) (string, error) {
	u := env.BaseURL + "/" + Repo + "/releases/latest"
	client := *env.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", remoteError("cannot reach GitHub", err)
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	const marker = "/releases/tag/"
	i := strings.LastIndex(loc, marker)
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", output.NewError(output.ExitRemote, fmt.Sprintf("cannot find the latest release: %s answered %s", u, resp.Status), "")
	}
	return loc[i+len(marker):], nil
}

// Download fetches the archive of plan, checks the signature and the checksum,
// and returns the openarch binary from the archive.
func Download(ctx context.Context, env Env, plan Plan) ([]byte, error) {
	tag := "v" + plan.Target
	base := env.BaseURL + "/" + Repo + "/releases/download/" + tag + "/"
	sums, err := get(ctx, env, base+ChecksumsFile)
	if err != nil {
		return nil, err
	}
	bundleJSON, err := get(ctx, env, base+BundleFile)
	if err != nil {
		return nil, err
	}
	if err := env.Verify(ctx, tag, sums, bundleJSON); err != nil {
		return nil, err
	}
	want, err := checksumOf(sums, plan.Asset)
	if err != nil {
		return nil, err
	}
	archive, err := get(ctx, env, base+plan.Asset)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, output.NewError(output.ExitError, "the checksum of "+plan.Asset+" does not match checksums.txt", "Nothing was changed. Try again later.")
	}
	return extract(archive, plan.Asset)
}

// VerifySigstore checks bundleJSON against the public Sigstore instance.
func VerifySigstore(ctx context.Context, tag string, sums, bundleJSON []byte) error {
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return signatureError(fmt.Errorf("cannot read %s: %w", BundleFile, err))
	}
	v, err := publicVerifier(ctx)
	if err != nil {
		return err
	}
	return verifyEntity(v, tag, sums, &b)
}

// verifyEntity checks that e signs sums, with a certificate for the release
// workflow at tag, issued to GitHub Actions.
func verifyEntity(v *verify.Verifier, tag string, sums []byte, e verify.SignedEntity) error {
	id, err := verify.NewShortCertificateIdentity(Issuer, "", Identity(tag), "")
	if err != nil {
		return err
	}
	if _, err := v.Verify(e, verify.NewPolicy(verify.WithArtifact(bytes.NewReader(sums)), verify.WithCertificateIdentity(id))); err != nil {
		return signatureError(err)
	}
	return nil
}

func signatureError(err error) error {
	return &output.Error{Code: output.ExitError, Message: "the signature of checksums.txt is not valid: " + err.Error(),
		Hint: "Nothing was changed. Do not install this release by hand either.", Err: err}
}

func remoteError(msg string, err error) error {
	return &output.Error{Code: output.ExitRemote, Message: msg + ": " + err.Error(), Err: err}
}

func get(ctx context.Context, env Env, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := env.HTTP.Do(req)
	if err != nil {
		return nil, remoteError("cannot download "+path.Base(u), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		hint := ""
		if resp.StatusCode == http.StatusNotFound && path.Base(u) == BundleFile {
			hint = "Releases before 0.2.0 have no Sigstore bundle. Install them by hand."
		}
		return nil, output.NewError(output.ExitRemote, fmt.Sprintf("cannot download %s: %s", path.Base(u), resp.Status), hint)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, remoteError("cannot download "+path.Base(u), err)
	}
	if len(data) > maxDownload {
		return nil, output.NewError(output.ExitRemote, path.Base(u)+" is too large", "")
	}
	return data, nil
}

// checksumOf finds the SHA-256 of name in a checksums.txt file.
func checksumOf(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", output.NewError(output.ExitRemote, name+" is not in checksums.txt", "This release may have no build for this platform.")
}

// extract returns the openarch binary from a .tar.gz or .zip archive.
func extract(archive []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == "openarch.exe" {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return io.ReadAll(io.LimitReader(rc, maxDownload))
			}
		}
		return nil, errors.New("openarch.exe is not in " + name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("openarch is not in " + name)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == "openarch" && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, maxDownload))
		}
	}
}

// Install replaces exe with bin. The new file is written next to exe and then
// renamed, so exe is never half written. Windows cannot replace a running
// executable, so there the old one is first renamed to exe.old.
func Install(exe string, bin []byte, goos string) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".openarch-update-*")
	if err != nil {
		return installError(exe, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return installError(exe, err)
	}
	if err := tmp.Close(); err != nil {
		return installError(exe, err)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return installError(exe, err)
	}
	if goos == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return installError(exe, err)
		}
		if err := os.Rename(tmp.Name(), exe); err != nil {
			_ = os.Rename(old, exe)
			return installError(exe, err)
		}
		return nil
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return installError(exe, err)
	}
	return nil
}

// RemoveOld deletes the exe.old file that an update on Windows leaves behind.
func RemoveOld(exe string) {
	_ = os.Remove(exe + ".old")
}

func installError(exe string, err error) error {
	hint := ""
	if errors.Is(err, os.ErrPermission) {
		hint = "You cannot write to " + filepath.Dir(exe) + ". Run the command again with the rights to change that folder."
	}
	return &output.Error{Code: output.ExitError, Message: "cannot replace " + exe + ": " + err.Error(), Hint: hint, Err: err}
}
