package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func newDoctorCmd(app *App) *cobra.Command {
	var opts doctor.Options
	ports := make([]string, 0, len(doctor.DefaultPorts))
	for _, p := range doctor.DefaultPorts {
		ports = append(ports, strconv.Itoa(p))
	}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the prerequisites to run OpenArch",
		Long: `Check the prerequisites to run OpenArch: Docker, Docker Compose,
Docker resources, free ports and access to the image registry.

Each check reports ok, warn, fail or skip, and a fix when something is wrong.
Exit code 3 means at least one check failed. Warnings do not change the exit code.

Checks: ` + strings.Join(doctor.IDs(app.doctorEnv(), doctor.Options{}), ", ") + `
(linux.max_map_count runs only on Linux; one port.<n> check runs for each --port)`,
		Example: `  openarch doctor
  openarch doctor --output json
  openarch doctor --check docker.daemon --check docker.memory
  openarch doctor --port 8080 --port 9200`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := doctor.Run(cmd.Context(), app.doctorEnv(), opts)
			if err != nil {
				return err
			}
			if err := app.Printer.Print(report); err != nil {
				return err
			}
			if report.Failed() {
				return output.Exit(output.ExitPrerequisite, "prerequisite check failed")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&opts.Only, "check", nil, "run only this check (repeatable)")
	cmd.Flags().IntSliceVar(&opts.Ports, "port", nil, "host port that must be free (repeatable, default "+strings.Join(ports, ",")+")")
	return cmd
}
