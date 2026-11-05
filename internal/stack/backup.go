package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Backup file names.
const (
	ManifestFile   = "manifest.json"
	MongoFile      = "mongodb.archive.gz"
	FilesFile      = "files.tar.gz"
	OpenSearchFile = "opensearch.tar.gz"
	// BackupsDir is the default folder of backups in a workspace.
	BackupsDir = "backups"
	// IndexPattern matches the OpenSearch indices of OpenArch (index name "nuxeo" and its
	// _configuration, _embeddings and _vocabularies companions, the OpenArch defaults).
	IndexPattern = "nuxeo*"
	// BackupFormat is the version of the backup layout.
	BackupFormat = 1
)

const (
	snapshotDir  = "/usr/share/opensearch/snapshots/openarch-cli"
	openarchData = "/deployments/nxserver/data"
)

// BackupFile is one file of a backup.
type BackupFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest describes a backup.
type Manifest struct {
	Format           int          `json:"format"`
	Created          time.Time    `json:"created"`
	Workspace        string       `json:"workspace"`
	OpenArchVersion  string       `json:"openarchVersion"`
	InsecureBackends bool         `json:"insecureBackends,omitempty"`
	Files            []BackupFile `json:"files"`
}

// BackupResult is the outcome of Backup.
type BackupResult struct {
	Path     string   `json:"path"`
	Manifest Manifest `json:"manifest"`
}

// WriteText writes the human-readable form.
func (r BackupResult) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Backup written to %s\n\n", r.Path)
	for _, f := range r.Manifest.Files {
		fmt.Fprintf(&b, "  %-20s %s\n", f.Name, humanSize(f.Size))
	}
	b.WriteString("\nThe backup holds all the data. Keep it private.\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// mongoAuth writes the root password from the container secret into a
// mongo tools config file, so it is never a process argument.
func (w Workspace) mongoAuth() (setup, args, cleanup string) {
	if !w.Config.Security.BackendAuth {
		return "", "", ""
	}
	return `umask 077; printf 'password: %s\n' "$(cat /run/secrets/mongodb_root_password)" > /tmp/openarch-cli.yml || exit 1; `,
		" --config /tmp/openarch-cli.yml --username root --authenticationDatabase admin",
		"; rc=$?; rm -f /tmp/openarch-cli.yml; exit $rc"
}

// osCurl defines a shell function "os" that calls OpenSearch with the
// OpenArch user from the container secret. printf is a shell builtin, so the
// password is never a process argument.
func (w Workspace) osCurl() string {
	if !w.Config.Security.BackendAuth {
		return `os() { curl -fsS "$@"; }; `
	}
	return `os() { printf 'user = "openarch:%s"\n' "$(cat /run/secrets/opensearch_app_password)" | curl -fsS -K - "$@"; }; `
}

func (w Workspace) running(ctx context.Context, env Env, services ...string) error {
	cs, err := w.runner(env).Ps(ctx)
	if err != nil {
		return err
	}
	for _, s := range services {
		ok := false
		for _, c := range cs {
			if c.Service == s && c.State == "running" {
				ok = true
			}
		}
		if !ok {
			return output.NewError(output.ExitUnhealthy, s+" is not running", "Start the stack with 'openarch up'.")
		}
	}
	return nil
}

// Backup writes a backup of MongoDB, the OpenArch files and the OpenSearch
// indices into a new folder in parent. The stack must be running.
func Backup(ctx context.Context, env Env, w Workspace, parent string, progress io.Writer) (BackupResult, error) {
	if err := w.running(ctx, env, "mongodb", "opensearch", "openarch-app"); err != nil {
		return BackupResult{}, err
	}
	now := time.Now().UTC()
	dir := filepath.Join(parent, now.Format("20060102T150405Z"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BackupResult{}, err
	}
	m := Manifest{Format: BackupFormat, Created: now, Workspace: w.Config.Name, OpenArchVersion: w.Config.Version, InsecureBackends: !w.Config.Security.BackendAuth}
	r := w.runner(env)
	setup, auth, cleanup := w.mongoAuth()
	steps := []struct {
		file, service, script string
	}{
		{MongoFile, "mongodb", setup + "mongodump --quiet" + auth + " --db openarch --archive --gzip" + cleanup},
		{FilesFile, "openarch-app", "tar czf - -C " + openarchData + " ."},
		{OpenSearchFile, "opensearch", w.osCurl() + `set -e
d=` + snapshotDir + `
rm -rf "$d"; mkdir -p "$d"
os -X PUT localhost:9200/_snapshot/openarch-cli -H 'Content-Type: application/json' -d "{\"type\":\"fs\",\"settings\":{\"location\":\"$d\"}}" >/dev/null
os -X PUT 'localhost:9200/_snapshot/openarch-cli/backup?wait_for_completion=true' -H 'Content-Type: application/json' \
  -d '{"indices":"` + IndexPattern + `","include_global_state":false}' | grep -q '"state":"SUCCESS"' || { echo "OpenSearch snapshot failed" >&2; exit 1; }
tar czf - -C "$d" .
os -X DELETE localhost:9200/_snapshot/openarch-cli >/dev/null
rm -rf "$d"`},
	}
	for _, s := range steps {
		_, _ = fmt.Fprintf(progress, "Backing up %s to %s\n", s.service, s.file)
		f, err := writeStream(dir, s.file, func(out io.Writer) error {
			err := r.RunIO(ctx, nil, out, progress, "exec", "-T", s.service, "sh", "-c", s.script)
			if err != nil {
				return fmt.Errorf("back up %s: %w", s.service, err)
			}
			return nil
		})
		if err != nil {
			return BackupResult{}, err
		}
		m.Files = append(m.Files, f)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return BackupResult{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), append(data, '\n'), 0o600); err != nil {
		return BackupResult{}, err
	}
	return BackupResult{Path: dir, Manifest: m}, nil
}

// writeStream writes a backup file with mode 0600 and returns its size and hash.
func writeStream(dir, name string, fill func(io.Writer) error) (BackupFile, error) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return BackupFile{}, err
	}
	h := sha256.New()
	cw := &countWriter{w: io.MultiWriter(f, h)}
	err = fill(cw)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && cw.n == 0 {
		err = fmt.Errorf("back up %s: no data", name)
	}
	if err != nil {
		return BackupFile{}, err
	}
	return BackupFile{Name: name, Size: cw.n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// ReadManifest reads and checks a backup: every file must exist and match its hash.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return m, output.Usage("not a backup: "+err.Error(), "Give the folder that 'openarch backup' printed.")
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, output.Usage("invalid "+ManifestFile+": "+err.Error(), "")
	}
	if m.Format != BackupFormat {
		return m, output.Usage(fmt.Sprintf("backup format %d is not supported", m.Format), fmt.Sprintf("This CLI reads format %d.", BackupFormat))
	}
	for _, want := range []string{MongoFile, FilesFile, OpenSearchFile} {
		i := -1
		for j, f := range m.Files {
			if f.Name == want {
				i = j
			}
		}
		if i < 0 {
			return m, output.Usage("the backup has no "+want, "")
		}
		if err := checkHash(filepath.Join(dir, want), m.Files[i].SHA256); err != nil {
			return m, err
		}
	}
	return m, nil
}

func checkHash(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return output.Usage("the backup is incomplete: "+err.Error(), "")
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return output.Usage("the backup file "+filepath.Base(path)+" is damaged (wrong SHA-256)", "Use another backup.")
	}
	return nil
}

// RestoreResult is the outcome of Restore.
type RestoreResult struct {
	Backup   string   `json:"backup"`
	Manifest Manifest `json:"manifest"`
	Status   Status   `json:"status"`
}

// WriteText writes the human-readable form.
func (r RestoreResult) WriteText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Restored the backup of %s (OpenArch %s).\n\n", r.Manifest.Created.Format(time.RFC3339), r.Manifest.OpenArchVersion); err != nil {
		return err
	}
	return r.Status.WriteText(w)
}

// Restore replaces the data of the workspace with a backup, then starts the stack.
// It checks the backup before it changes anything.
func Restore(ctx context.Context, env Env, w Workspace, dir string, opts UpOptions) (RestoreResult, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return RestoreResult{}, err
	}
	if m.InsecureBackends != !w.Config.Security.BackendAuth {
		// Data moves fine between modes; only the access changes.
		_, _ = fmt.Fprintln(opts.Progress, "Note: the backup comes from a workspace with other backend security settings.")
	}
	r := w.runner(env)
	p := opts.Progress
	// OpenArch must not write while its data is replaced.
	if err := r.Run(ctx, p, p, "stop", "openarch-app"); err != nil {
		return RestoreResult{}, fmt.Errorf("stop openarch-app: %w", err)
	}
	// 'up --wait' fails when a one-shot service is named on the command line, even
	// when it exits with 0. So the backends start first, then mongodb-init, which
	// starts the replica set of an empty MongoDB, runs on its own.
	if err := r.Run(ctx, p, p, "up", "--detach", "--wait", "--wait-timeout", fmt.Sprint(int(opts.Timeout.Seconds())), "mongodb", "opensearch"); err != nil {
		return RestoreResult{}, fmt.Errorf("start MongoDB and OpenSearch: %w", err)
	}
	if err := r.Run(ctx, p, p, "up", "--detach", "--no-deps", "mongodb-init"); err != nil {
		return RestoreResult{}, fmt.Errorf("start mongodb-init: %w", err)
	}
	if err := r.Run(ctx, p, p, "wait", "mongodb-init"); err != nil {
		return RestoreResult{}, fmt.Errorf("mongodb-init failed: %w", err)
	}
	setup, auth, cleanup := w.mongoAuth()
	steps := []struct {
		file, what string
		args       []string
	}{
		{MongoFile, "MongoDB", []string{"exec", "-T", "mongodb", "sh", "-c",
			setup + "mongorestore --quiet" + auth + " --drop --archive --gzip --nsInclude 'openarch.*'" + cleanup}},
		// openarch-app is stopped, so a one-off container with its volumes restores the files.
		{FilesFile, "OpenArch files", []string{"run", "--rm", "--no-deps", "-T", "--entrypoint", "sh", "openarch-app", "-c",
			"rm -rf " + openarchData + "/* " + openarchData + "/.[!.]* && tar xzf - -C " + openarchData}},
		{OpenSearchFile, "OpenSearch", []string{"exec", "-T", "opensearch", "sh", "-c", w.osCurl() + `set -e
d=` + snapshotDir + `
rm -rf "$d"; mkdir -p "$d"
tar xzf - -C "$d"
os -X PUT localhost:9200/_snapshot/openarch-cli -H 'Content-Type: application/json' -d "{\"type\":\"fs\",\"settings\":{\"location\":\"$d\"}}" >/dev/null
os -X DELETE 'localhost:9200/` + IndexPattern + `?ignore_unavailable=true&allow_no_indices=true' >/dev/null
os -X POST 'localhost:9200/_snapshot/openarch-cli/backup/_restore?wait_for_completion=true' -H 'Content-Type: application/json' \
  -d '{"indices":"` + IndexPattern + `","include_global_state":false}' | grep -q '"failed":0' || { echo "OpenSearch restore failed" >&2; exit 1; }
os -X DELETE localhost:9200/_snapshot/openarch-cli >/dev/null
rm -rf "$d"`}},
	}
	for _, s := range steps {
		_, _ = fmt.Fprintf(p, "Restoring %s from %s\n", s.what, s.file)
		f, err := os.Open(filepath.Join(dir, s.file))
		if err != nil {
			return RestoreResult{}, err
		}
		err = r.RunIO(ctx, f, p, p, s.args...)
		_ = f.Close()
		if err != nil {
			return RestoreResult{}, fmt.Errorf("restore %s: %w (the workspace data may be incomplete: restore again, or run 'openarch clean')", s.what, err)
		}
	}
	st, err := Up(ctx, env, w, UpOptions{SkipDoctor: true, Wait: true, Timeout: opts.Timeout, Progress: p})
	res := RestoreResult{Backup: dir, Manifest: m, Status: st}
	return res, err
}
