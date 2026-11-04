package cli

import (
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func TestDocumentFileUsageErrors(t *testing.T) {
	t.Setenv(envOutput, "")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"documents", "upload", "x.pdf"}, "--parent is required"},
		{[]string{"documents", "upload", "no-such-file.pdf", "--parent", "/"}, "cannot read the file"},
		{[]string{"documents", "content", "/a.pdf"}, "--out is required"},
		{[]string{"documents", "content", "/a.pdf", "--out", "-", "-o", "json"}, "cannot be used with --output json"},
		{[]string{"documents", "list", "/a", "/b"}, "accepts at most 1 argument"},
	} {
		// In JSON mode the error is on stdout.
		code, stdout, stderr := run(t, c.args...)
		if code != int(output.ExitUsage) || !strings.Contains(stdout+stderr, c.want) {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", c.args, code, stdout, stderr)
		}
	}
}
