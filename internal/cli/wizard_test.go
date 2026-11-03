package cli

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Every wizard question sets an init flag, and every option flag has a question.
func TestWizardQuestionsMapToFlags(t *testing.T) {
	cmd := newInitCmd(newApp(Streams{}))
	flags := wizardFlags()
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("question %q has no init flag", name)
		}
	}
	for _, name := range append(slices.Clone(optionFlags), "admin-password-stdin") {
		if !slices.Contains(flags, name) {
			t.Errorf("init flag --%s has no wizard question", name)
		}
	}
	seen := map[string]bool{}
	for _, name := range flags {
		if seen[name] {
			t.Errorf("flag %q is asked twice", name)
		}
		seen[name] = true
	}
}

// The accessible mode reads plain lines, so a whole wizard run can be scripted.
func TestWizardAccessibleRun(t *testing.T) {
	t.Setenv(envAccessible, "1")
	f := &initFlags{}
	cmd := initCmd(newApp(Streams{}), f)
	if err := cmd.Flags().Parse([]string{"--server-name", "oa.local", "--version", "1.0.90"}); err != nil {
		t.Fatal(err)
	}
	answers := strings.Join([]string{
		"2",       // storage: bind
		"/srv/oa", // data folder
		"2",       // tls: self-signed
		"",        // port: keep the default
		"1",       // listen on 127.0.0.1
		"3",       // expose: debug
		"0",       // confirm the selection
		"0",       // extra services: none
		"8g",      // memory
		// The password prompt needs a real terminal in accessible mode, so it gets no answer here.
	}, "\n") + "\n"
	var out bytes.Buffer
	pw, err := runWizard(context.Background(), cmd.Flags(), f, &lineReader{lines: strings.SplitAfter(answers, "\n")}, &out)
	if err != nil {
		t.Fatalf("runWizard: %v\n%s", err, out.String())
	}
	got := f.cfg
	if got.Storage.Mode != workspace.StorageBind || got.Storage.DataDir != "/srv/oa" || got.TLS.Mode != workspace.TLSSelfSigned ||
		got.Port != 8443 || got.BindIP != "127.0.0.1" || !slices.Equal(got.Expose, []string{"debug"}) ||
		len(got.With) != 0 || got.Memory != "8g" || got.ServerName != "oa.local" || pw != "" {
		t.Errorf("answers = %+v, password %q\noutput:\n%s", got, pw, out.String())
	}
	if strings.Contains(out.String(), "Server name") {
		t.Error("the wizard asked for a value given as a flag")
	}
}

// lineReader returns one line per Read, like a terminal in canonical mode.
// Each huh field reads with its own scanner, so a reader that returns all the
// input at once would give every answer to the first field.
type lineReader struct{ lines []string }

func (r *lineReader) Read(p []byte) (int, error) {
	if len(r.lines) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.lines[0])
	r.lines[0] = r.lines[0][n:]
	if r.lines[0] == "" {
		r.lines = r.lines[1:]
	}
	return n, nil
}

func TestEquivalentCommand(t *testing.T) {
	f := &initFlags{cfg: workspace.Default("")}
	f.cfg.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "/my data"}
	f.cfg.TLS.Mode, f.cfg.Port = workspace.TLSSelfSigned, 8443
	f.cfg.Expose = []string{"debug", "opensearch"}
	got := equivalentCommand("ws", f, true)
	want := "openarch init --dir ws --storage bind --data-dir '/my data' --tls self-signed --expose debug --expose opensearch --admin-password-stdin --non-interactive"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := equivalentCommand("", &initFlags{cfg: workspace.Default("")}, false); got != "openarch init --non-interactive" {
		t.Errorf("defaults: %s", got)
	}
}
