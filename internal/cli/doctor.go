package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/models"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func newDoctorCmd(app *App) *cobra.Command {
	var opts doctor.Options
	var m modelFlags
	ports := make([]string, 0, len(doctor.DefaultPorts))
	for _, p := range doctor.DefaultPorts {
		ports = append(ports, strconv.Itoa(p))
	}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the prerequisites to run OpenArch",
		Long: `Check the prerequisites to run OpenArch: Docker, Docker Compose,
Docker resources, free ports and access to the image registry. It also makes
one embedding with the model for vector search (see 'demo load --embeddings'),
and checks its size: OpenArch takes vectors of at most 1024 dimensions. When
no model API answers, that check is skipped.

Each check reports ok, warn, fail or skip, and a fix when something is wrong.
Exit code 3 means at least one check failed. Warnings do not change the exit code.

Checks: ` + strings.Join(doctor.IDs(app.doctorEnv(), doctor.Options{Model: models.EmbedModel()}), ", ") + `
(linux.max_map_count runs only on Linux; one port.<n> check runs for each --port)

Inside a workspace, the default ports are the ones openarch.yaml publishes.`,
		Example: `  openarch doctor
  openarch doctor --output json
  openarch doctor --check docker.daemon --check docker.memory
  openarch doctor --port 8080 --port 9200
  openarch doctor --check models.embedding --model huggingface.co/gpustack/bge-m3-gguf`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(opts.Ports) == 0 {
				ports, err := workspacePorts(app.Opts.Dir)
				if err != nil {
					return err
				}
				opts.Ports = ports
			}
			opts.Model, opts.ModelURL = m.model, m.url
			report, err := doctor.Run(cmd.Context(), app.doctorEnv(), opts)
			if err != nil {
				return err
			}
			if err := app.Printer.Print(report); err != nil {
				return err
			}
			if app.Printer.Format() != output.JSON {
				writeDoctorFooter(app.Streams.Out)
			}
			if report.Failed() {
				return output.Exit(output.ExitPrerequisite, "prerequisite check failed")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&opts.Only, "check", nil, "run only this check (repeatable)")
	m.register(cmd)
	cmd.Flags().IntSliceVar(&opts.Ports, "port", nil, "host port that must be free (repeatable, default: the workspace ports, or "+strings.Join(ports, ",")+")")
	return cmd
}

// workspacePorts returns the host ports of the workspace in dir, or nil when there is no workspace.
func workspacePorts(dir string) ([]int, error) {
	root, err := workspace.Find(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, workspace.ConfigFile)
	if _, err := os.Stat(path); err != nil {
		return nil, nil //nolint:nilerr // No workspace: doctor uses the default ports.
	}
	cfg, err := workspace.Load(path)
	if err != nil {
		return nil, err
	}
	var ports []int
	for _, p := range cfg.HostPorts() {
		ports = append(ports, p.Port)
	}
	return ports, nil
}
