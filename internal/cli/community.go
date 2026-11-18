package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

// communityURL is the Hyland OpenArch group on Hyland Community, where users ask usage questions.
// Bugs and feature requests go to the GitHub issues of the project.
const communityURL = "https://community.hyland.com/group/26-hyland-openarch"

// communityLine is the last line of text errors and of the doctor report.
const communityLine = "Questions? Ask in the Hyland OpenArch group: " + communityURL

// writeErrorHelp adds communityLine after a text error. It writes nothing in JSON mode (stdout holds
// exactly one JSON document), for usage errors (their hint is enough) and for silent errors.
func writeErrorHelp(w io.Writer, format output.Format, err error) {
	if format == output.JSON || output.CodeOf(err) == output.ExitUsage {
		return
	}
	var e *output.Error
	if errors.As(err, &e) && e.Silent {
		return
	}
	_, _ = fmt.Fprintln(w, communityLine)
}

// writeDoctorFooter ends the text doctor report with the version and platform, so that the whole
// output can be pasted into a question.
func writeDoctorFooter(w io.Writer) {
	v := version.Get()
	_, _ = fmt.Fprintf(w, "\nCLI for OpenArch %s (commit %s), %s\n%s\nPaste this output with your question.\n",
		v.Version, v.Commit, v.Platform, communityLine)
}
