package stack

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Image is the OpenArch image without its tag.
const Image = "ghcr.io/hyland/openarch-app-jvm"

// UpgradePlan describes an upgrade.
type UpgradePlan struct {
	Workspace string   `json:"workspace"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Steps     []string `json:"steps"`
	// Downgrade is true when To is an older version than From.
	Downgrade bool `json:"downgrade"`
}

// PlanUpgrade returns the steps of an upgrade to version, or a usage error.
func PlanUpgrade(w Workspace, version string, smoke bool) (UpgradePlan, error) {
	cfg := w.Config
	cfg.Version = version
	if err := cfg.Validate(); err != nil {
		return UpgradePlan{}, err
	}
	p := UpgradePlan{Workspace: w.Dir, From: w.Config.Version, To: version, Downgrade: CompareVersions(version, w.Config.Version) < 0}
	p.Steps = []string{
		"pull " + Image + ":" + version,
		"set version " + version + " in " + workspace.ConfigFile + ", and render the workspace files",
		"start the stack and wait until it is healthy",
	}
	if smoke {
		p.Steps = append(p.Steps, "run the smoke test")
	}
	return p, nil
}

// WriteText writes the human-readable form.
func (p UpgradePlan) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Dry run. Upgrade of %s from %s to %s:\n", p.Workspace, p.From, p.To)
	for i, s := range p.Steps {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, s)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// CompareVersions compares dotted numeric versions such as 1.0.55. It returns
// 0 when a version is not numeric (for example "latest"): the order is unknown.
func CompareVersions(a, b string) int {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return 0
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVersion(v string) ([]int, bool) {
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Upgrade pulls the new image, then writes the new version, renders the
// workspace and starts the stack. The workspace files change only after the
// pull worked, so a tag that does not exist changes nothing.
func Upgrade(ctx context.Context, env Env, gen generate.Env, w Workspace, version string, opts UpOptions) (Status, error) {
	if err := env.Compose(ctx, compose.Command{Args: []string{"pull", Image + ":" + version}, Stdout: opts.Progress, Stderr: opts.Progress}); err != nil {
		return Status{}, output.NewError(output.ExitError, fmt.Sprintf("cannot pull %s:%s: %v", Image, version, err),
			"Check the version. Tags are listed at https://github.com/hyland/OpenArch/pkgs/container/openarch-app-jvm.")
	}
	cfg := w.Config
	cfg.Version = version
	if err := writeConfig(w.Dir, cfg); err != nil {
		return Status{}, err
	}
	if _, err := generate.Render(ctx, gen, generate.RenderOptions{Dir: w.Dir}); err != nil {
		return Status{}, err
	}
	w2, err := Load(w.Dir)
	if err != nil {
		return Status{}, err
	}
	return Up(ctx, env, w2, opts)
}

func writeConfig(dir string, cfg workspace.Config) error {
	return writeFile(filepath.Join(dir, workspace.ConfigFile), cfg.Marshal(), 0o644)
}
