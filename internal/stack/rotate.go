package stack

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Secrets that 'secrets rotate' can change.
const (
	RotateAdmin      = "admin"        // OpenArch admin
	RotateMongoRoot  = "mongodb-root" // MongoDB root user
	RotateMongoApp   = "mongodb-app"  // MongoDB user of OpenArch
	RotateOpenSearch = "opensearch"   // OpenSearch users admin, openarch and kibanaserver
)

// RotateTargets lists the valid targets, in the order they are rotated.
var RotateTargets = []string{RotateMongoRoot, RotateMongoApp, RotateOpenSearch, RotateAdmin}

// RotateSkip is a target that was not rotated, and why.
type RotateSkip struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// RotateResult is the outcome of Rotate. It never holds a password.
type RotateResult struct {
	Workspace string       `json:"workspace"`
	Rotated   []string     `json:"rotated"`
	Skipped   []RotateSkip `json:"skipped,omitempty"`
	// Applied is false when the stack was not running: the new passwords apply at the next 'openarch up'.
	Applied bool    `json:"applied"`
	Status  *Status `json:"status,omitempty"`
}

// WriteText writes the human-readable form.
func (r RotateResult) WriteText(w io.Writer) error {
	var b strings.Builder
	if len(r.Rotated) == 0 {
		b.WriteString("No password was changed.\n")
	} else {
		fmt.Fprintf(&b, "New passwords for: %s. They are in .env.local.\n", strings.Join(r.Rotated, ", "))
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(&b, "Skipped %s: %s\n", s.Target, s.Reason)
	}
	if len(r.Rotated) > 0 && !r.Applied {
		b.WriteString("The stack is not running: the new passwords apply at the next 'openarch up'.\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	if r.Status != nil {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		return r.Status.WriteText(w)
	}
	return nil
}

// RotatePlan checks the targets and returns the ones to rotate and the ones to skip.
func RotatePlan(w Workspace, targets []string) ([]string, []RotateSkip, error) {
	if len(targets) == 0 {
		targets = RotateTargets
	}
	for _, t := range targets {
		if !slices.Contains(RotateTargets, t) {
			return nil, nil, output.Usage(fmt.Sprintf("unknown secret %q", t), "Valid values: "+strings.Join(RotateTargets, ", ")+".")
		}
	}
	var do []string
	var skip []RotateSkip
	for _, t := range RotateTargets {
		if !slices.Contains(targets, t) {
			continue
		}
		switch {
		case t == RotateAdmin && w.Config.Security.DefaultUsers:
			skip = append(skip, RotateSkip{t, "the workspace uses the users bundled in the OpenArch image"})
		case t != RotateAdmin && !w.Config.Security.BackendAuth:
			skip = append(skip, RotateSkip{t, "MongoDB and OpenSearch run with no authentication (security.backendAuth is false)"})
		default:
			do = append(do, t)
		}
	}
	return do, skip, nil
}

// Rotate gives new passwords to the selected secrets. Each live change is made
// first, and .env.local is saved right after it, so the file always matches
// what the services accept. Then the workspace is rendered and, when the stack
// runs, 'up' recreates the services whose secrets changed.
func Rotate(ctx context.Context, env Env, gen generate.Env, w Workspace, targets []string, opts UpOptions) (RotateResult, error) {
	do, skip, err := RotatePlan(w, targets)
	if err != nil {
		return RotateResult{}, err
	}
	res := RotateResult{Workspace: w.Dir, Rotated: []string{}, Skipped: skip}
	live := slices.Contains(do, RotateMongoRoot) || slices.Contains(do, RotateOpenSearch)
	runningErr := w.running(ctx, env, "mongodb", "opensearch", "openarch-app")
	if live && runningErr != nil {
		return res, output.NewError(output.ExitUnhealthy, "the MongoDB root and OpenSearch passwords can only be changed on a running stack",
			"Start it with 'openarch up', or rotate only --only "+RotateMongoApp+","+RotateAdmin+".")
	}
	s := w.Secrets
	save := func(target string) error {
		if err := writeFile(filepath.Join(w.Dir, localenv.File), s.Marshal(), 0o600); err != nil {
			return fmt.Errorf("the %s password changed, but .env.local could not be saved: %w", target, err)
		}
		res.Rotated = append(res.Rotated, target)
		return nil
	}
	r := w.runner(env)
	p := opts.Progress
	for _, t := range do {
		switch t {
		case RotateMongoRoot:
			pw := localenv.Password(localenv.PasswordLength)
			script := `read -r NEW || exit 1; export NEW; mongosh --quiet --nodb --eval '
const fs = require("fs");
const old = fs.readFileSync("/run/secrets/mongodb_root_password", "utf8").trim();
const conn = new Mongo("mongodb://root:" + encodeURIComponent(old) + "@localhost:27017/admin?directConnection=true");
conn.getDB("admin").changeUserPassword("root", process.env.NEW);'`
			if err := r.RunIO(ctx, strings.NewReader(pw+"\n"), p, p, "exec", "-T", "mongodb", "sh", "-c", script); err != nil {
				return res, fmt.Errorf("change the MongoDB root password: %w", err)
			}
			s.MongoRootPassword = pw
		case RotateMongoApp:
			// mongodb-init sets it at the next 'up', which recreates it with the new secret.
			s.MongoAppPassword = localenv.Password(localenv.PasswordLength)
		case RotateOpenSearch:
			// The admin changes the other two first, then itself.
			for _, u := range []struct {
				name string
				pw   *string
			}{
				{localenv.OpenSearchAppUser, &s.OpenSearchAppPassword},
				{localenv.DashboardsUser, &s.DashboardsPassword},
				{localenv.OpenSearchAdminUser, &s.OpenSearchAdminPassword},
			} {
				pw := localenv.Password(localenv.PasswordLength)
				cfg := fmt.Sprintf("user = \"%s:%s\"\ndata = \"{\\\"password\\\":\\\"%s\\\"}\"\n", localenv.OpenSearchAdminUser, s.OpenSearchAdminPassword, pw)
				script := "curl -fsS -o /dev/null -K - -X PUT -H 'Content-Type: application/json' localhost:9200/_plugins/_security/api/internalusers/" + u.name
				if err := r.RunIO(ctx, strings.NewReader(cfg), p, p, "exec", "-T", "opensearch", "sh", "-c", script); err != nil {
					return res, fmt.Errorf("change the OpenSearch password of %s: %w (with a workspace created before the users were unreserved, create the OpenSearch data again)", u.name, err)
				}
				*u.pw = pw
				if err := writeFile(filepath.Join(w.Dir, localenv.File), s.Marshal(), 0o600); err != nil {
					return res, fmt.Errorf("the OpenSearch password of %s changed, but .env.local could not be saved: %w", u.name, err)
				}
			}
		case RotateAdmin:
			s.AdminPassword = localenv.Password(localenv.PasswordLength)
		}
		if err := save(t); err != nil {
			return res, err
		}
	}
	if len(res.Rotated) == 0 {
		return res, nil
	}
	if _, err := generate.Render(ctx, gen, generate.RenderOptions{Dir: w.Dir}); err != nil {
		return res, err
	}
	if runningErr != nil {
		return res, nil
	}
	w2, err := Load(w.Dir)
	if err != nil {
		return res, err
	}
	st, err := Up(ctx, env, w2, UpOptions{SkipDoctor: true, Wait: true, Timeout: opts.Timeout, Progress: p})
	res.Applied, res.Status = err == nil, &st
	return res, err
}
