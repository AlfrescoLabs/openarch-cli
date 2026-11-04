package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseBundle checks the signature of a real release in
// OPENARCH_RELEASE_DIR (the GoReleaser dist folder) for OPENARCH_RELEASE_TAG,
// with the public Sigstore instance. The release workflow runs it.
func TestReleaseBundle(t *testing.T) {
	dir, tag := os.Getenv("OPENARCH_RELEASE_DIR"), os.Getenv("OPENARCH_RELEASE_TAG")
	if dir == "" || tag == "" {
		t.Skip("OPENARCH_RELEASE_DIR and OPENARCH_RELEASE_TAG are not set")
	}
	sums, err := os.ReadFile(filepath.Join(dir, ChecksumsFile))
	if err != nil {
		t.Fatal(err)
	}
	bundleJSON, err := os.ReadFile(filepath.Join(dir, BundleFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySigstore(context.Background(), tag, sums, bundleJSON); err != nil {
		t.Fatal(err)
	}
	if err := VerifySigstore(context.Background(), tag+"-other", sums, bundleJSON); err == nil {
		t.Fatal("the bundle is accepted for another tag")
	}
}
