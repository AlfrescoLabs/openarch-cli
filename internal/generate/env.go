package generate

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SystemEnv returns an Env backed by the real system.
func SystemEnv() Env {
	return Env{HostMemory: dockerMemory, Now: time.Now}
}

// dockerMemory returns the memory reported by 'docker info'. On Docker Desktop
// this is the memory of its VM, not of the host.
func dockerMemory(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.MemTotal}}").Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}
