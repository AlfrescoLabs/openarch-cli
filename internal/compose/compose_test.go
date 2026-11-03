package compose

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParsePs(t *testing.T) {
	lines := []byte(`{"Name":"ws-a-1","Service":"a","State":"running","Health":"healthy","ExitCode":0,"Publishers":[{"PublishedPort":8080,"TargetPort":8080}]}
{"Name":"ws-b-1","Service":"b","State":"exited","ExitCode":1,"Publishers":[]}
`)
	cs, err := ParsePs(lines)
	if err != nil || len(cs) != 2 || cs[0].Publishers[0].PublishedPort != 8080 || cs[1].ExitCode != 1 {
		t.Fatalf("ParsePs = %+v, %v", cs, err)
	}
	array, err := ParsePs([]byte(`[{"Service":"a","State":"running"}]`))
	if err != nil || len(array) != 1 {
		t.Errorf("ParsePs(array) = %+v, %v", array, err)
	}
	if cs, err := ParsePs([]byte("\n")); err != nil || cs != nil {
		t.Errorf("ParsePs(empty) = %+v, %v", cs, err)
	}
	if _, err := ParsePs([]byte("not json")); err == nil {
		t.Error("ParsePs accepted bad input")
	}
}

func TestPsSkipsOneOffContainers(t *testing.T) {
	r := Runner{Dir: t.TempDir(), Exec: func(_ context.Context, c Command) error {
		_, err := c.Stdout.Write([]byte(`{"Service":"a","State":"running","Labels":"com.docker.compose.oneoff=False"}
{"Service":"a","State":"exited","Labels":"x=y,com.docker.compose.oneoff=True"}
`))
		return err
	}}
	cs, err := r.Ps(context.Background())
	if err != nil || len(cs) != 1 || cs[0].State != "running" {
		t.Errorf("Ps = %+v, %v", cs, err)
	}
}

func TestEnvFiles(t *testing.T) {
	dir := t.TempDir()
	var got []string
	r := Runner{Dir: dir, Env: []string{"A=1"}, Exec: func(_ context.Context, c Command) error {
		got = c.Args
		if !slices.Equal(c.Env, []string{"A=1"}) {
			t.Errorf("env = %v", c.Env)
		}
		return nil
	}}
	_ = r.Run(context.Background(), nil, nil, "ps")
	if slices.Contains(got, filepath.Join(dir, ".env.local")) {
		t.Errorf("args name a missing .env.local: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = r.Run(context.Background(), nil, nil, "ps")
	if !slices.Contains(got, filepath.Join(dir, ".env.local")) || got[len(got)-1] != "ps" {
		t.Errorf("args = %v", got)
	}
}
