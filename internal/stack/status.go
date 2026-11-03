// Package stack starts, stops and checks the Docker Compose stack of a workspace.
package stack

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Stack states.
const (
	Healthy   = "healthy"
	Starting  = "starting"
	Unhealthy = "unhealthy"
	Stopped   = "stopped"
)

// ServiceStatus is the state of one service.
type ServiceStatus struct {
	Name string `json:"name"`
	// State is the container state from Docker (running, exited, ...), or "missing".
	State string `json:"state"`
	// Health is the Docker healthcheck result (healthy, starting, unhealthy), or "" when the service has none.
	Health   string `json:"health,omitempty"`
	ExitCode int    `json:"exitCode"`
	OneShot  bool   `json:"oneShot"`
	Ports    []int  `json:"ports,omitempty"`
	// OK reports whether this service is in its expected final state.
	OK bool `json:"ok"`
}

// APICheck is the result of an HTTP request to the OpenArch health endpoint.
type APICheck struct {
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Endpoint is a URL a user can open.
type Endpoint struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Status is the state of the whole stack.
type Status struct {
	Workspace string          `json:"workspace"`
	State     string          `json:"state"`
	Services  []ServiceStatus `json:"services"`
	API       *APICheck       `json:"api,omitempty"`
	Endpoints []Endpoint      `json:"endpoints"`
	AdminUser string          `json:"adminUser,omitempty"`
	// Command is the CLI command that produced the status, for the text form.
	Command string `json:"-"`
}

// Evaluate combines the containers and the API check into a status.
// api is nil when the check did not run.
func Evaluate(cfg workspace.Config, containers []compose.Container, api *APICheck) Status {
	st := Status{API: api, Endpoints: Endpoints(cfg)}
	byService := map[string]compose.Container{}
	for _, c := range containers {
		byService[c.Service] = c
	}
	running, starting, bad := 0, false, false
	for _, svc := range cfg.Services() {
		s := ServiceStatus{Name: svc.Name, OneShot: svc.OneShot, State: "missing"}
		if c, ok := byService[svc.Name]; ok {
			s.State, s.Health, s.ExitCode = c.State, c.Health, c.ExitCode
			for _, p := range c.Publishers {
				if p.PublishedPort != 0 && !slices.Contains(s.Ports, p.PublishedPort) {
					s.Ports = append(s.Ports, p.PublishedPort)
				}
			}
		}
		switch {
		case svc.OneShot:
			s.OK = s.State == "exited" && s.ExitCode == 0
			if s.State == "running" || s.State == "created" {
				starting = true
			} else if s.State == "exited" && s.ExitCode != 0 {
				bad = true
			}
		case s.State == "running":
			running++
			switch s.Health {
			case "", "healthy":
				s.OK = true
			case "starting":
				starting = true
			default:
				bad = true
			}
		case s.State == "restarting" || s.State == "created":
			// A created container waits for the services it depends on.
			starting = true
		case s.State != "missing":
			bad = true
		}
		st.Services = append(st.Services, s)
	}
	switch {
	case len(containers) == 0 || (running == 0 && !starting):
		st.State = Stopped
		if bad {
			st.State = Unhealthy
		}
	case bad:
		st.State = Unhealthy
	case starting:
		st.State = Starting
	default:
		st.State = Healthy
		for _, s := range st.Services {
			if !s.OK {
				st.State = Unhealthy
			}
		}
		if st.State == Healthy && api != nil && !api.OK {
			st.State = Unhealthy
		}
	}
	return st
}

// Endpoints returns the URLs of the workspace.
func Endpoints(cfg workspace.Config) []Endpoint {
	u := cfg.URL()
	e := []Endpoint{
		{Name: "api", URL: u},
		{Name: "swagger", URL: u + "/swagger-ui/index.html"},
	}
	host := cfg.ServerName
	if cfg.Has(workspace.WithDashboards) {
		e = append(e, Endpoint{Name: "dashboards", URL: "http://" + host + ":5601"})
	}
	if cfg.Has(workspace.ExposeOpenSearch) {
		e = append(e, Endpoint{Name: "opensearch", URL: "http://" + host + ":9200"})
	}
	if cfg.Has(workspace.ExposeMongoDB) {
		e = append(e, Endpoint{Name: "mongodb", URL: "mongodb://" + host + ":27017/?directConnection=true"})
	}
	if cfg.Has(workspace.ExposeDebug) {
		e = append(e, Endpoint{Name: "debug", URL: host + ":5005"})
	}
	return e
}

// WriteText writes the human-readable form.
func (s Status) WriteText(w io.Writer) error {
	var b strings.Builder
	switch s.Command {
	case "up":
		fmt.Fprintf(&b, "OpenArch is %s.\n\n", s.State)
	default:
		fmt.Fprintf(&b, "Workspace %s: %s\n\n", s.Workspace, s.State)
	}
	width := 0
	for _, svc := range s.Services {
		width = max(width, len(svc.Name))
	}
	for _, svc := range s.Services {
		mark := "[ok]"
		if !svc.OK {
			mark = "[--]"
		}
		state := svc.State
		if svc.Health != "" {
			state += " (" + svc.Health + ")"
		}
		if svc.OneShot && svc.State == "exited" {
			state = fmt.Sprintf("exited with code %d", svc.ExitCode)
		}
		fmt.Fprintf(&b, "%-4s %-*s  %s\n", mark, width, svc.Name, state)
	}
	if s.API != nil {
		if s.API.OK {
			fmt.Fprintf(&b, "\nAPI health: ok (%s)\n", s.API.URL)
		} else {
			reason := s.API.Error
			if reason == "" {
				reason = fmt.Sprintf("HTTP %d", s.API.StatusCode)
			}
			fmt.Fprintf(&b, "\nAPI health: failed (%s): %s\n", s.API.URL, reason)
		}
	}
	if s.State != Stopped {
		b.WriteString("\n")
		for _, e := range s.Endpoints {
			fmt.Fprintf(&b, "%-10s %s\n", e.Name, e.URL)
		}
		if s.AdminUser != "" {
			fmt.Fprintf(&b, "\nAdmin user: %s (password in .env.local)\n", s.AdminUser)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
