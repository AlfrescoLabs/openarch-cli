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

func TestParseHealth(t *testing.T) {
	data := []byte(`[
  {"Config": {"Labels": {"com.docker.compose.service": "opensearch"}},
   "State": {"Health": {"Status": "starting", "Log": [
     {"Start": "2026-11-09T10:00:00.123456789Z", "ExitCode": 22, "Output": "curl: (22) The requested URL returned error: 401\n"}]}}},
  {"Config": {"Labels": {"com.docker.compose.service": "mongodb-init"}}, "State": {}}
]`)
	hs, err := ParseHealth(data)
	if err != nil || len(hs) != 1 {
		t.Fatalf("ParseHealth = %+v, %v", hs, err)
	}
	h := hs[0]
	if h.Service != "opensearch" || h.Status != "starting" || len(h.Log) != 1 || h.Log[0].ExitCode != 22 || h.Log[0].Start.Minute() != 0 {
		t.Errorf("health = %+v", h)
	}
	if _, err := ParseHealth([]byte("not json")); err == nil {
		t.Error("ParseHealth accepted bad JSON")
	}
}
