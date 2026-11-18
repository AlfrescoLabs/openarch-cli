package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func TestWriteErrorHelp(t *testing.T) {
	tests := []struct {
		name   string
		format output.Format
		err    error
		want   bool
	}{
		{"remote error in text", output.Text, output.NewError(output.ExitRemote, "boom", ""), true},
		{"plain error in text", output.Text, errors.New("boom"), true},
		{"usage error", output.Text, output.Usage("bad flag", "see help"), false},
		{"json mode", output.JSON, output.NewError(output.ExitRemote, "boom", ""), false},
		{"silent error", output.Text, output.Exit(output.ExitPrerequisite, "failed"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			writeErrorHelp(&b, tt.format, tt.err)
			if got := b.String() == communityLine+"\n"; got != tt.want {
				t.Errorf("wrote %q, want the community line: %v", b.String(), tt.want)
			}
		})
	}
}

func TestDoctorFooter(t *testing.T) {
	code, stdout, stderr := runDoctor(t, false)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\nCLI for OpenArch ") || !strings.HasSuffix(stdout, communityLine+"\nPaste this output with your question.\n") {
		t.Errorf("stdout has no footer: %q", stdout)
	}
	// The footer is text only: JSON stdout stays one document (TestDoctorFailureJSON).
	_, jsonOut, _ := runDoctor(t, false, "--output", "json")
	if strings.Contains(jsonOut, communityURL) {
		t.Errorf("JSON output has the footer: %q", jsonOut)
	}
}
