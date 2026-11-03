package workspace

// Service is a service of the rendered compose.yaml.
type Service struct {
	Name string
	// OneShot services run once and exit. Exit code 0 is their healthy state.
	OneShot bool
	// HashVar is the environment variable that 'openarch up' sets to a hash of
	// the files and secrets the service reads at start. The service has a label
	// with its value, so a change recreates the container. Empty when unused.
	HashVar string
}

// Hash variables used in compose.yaml labels.
const (
	HashVarApp        = "OPENARCH_APP_CONFIG_HASH"
	HashVarOpenSearch = "OPENARCH_OPENSEARCH_CONFIG_HASH"
	HashVarDashboards = "OPENARCH_DASHBOARDS_CONFIG_HASH"
	HashVarProxy      = "OPENARCH_PROXY_CONFIG_HASH"
)

// Services returns the services that compose.yaml defines for c, in start order.
func (c Config) Services() []Service {
	var s []Service
	if c.Storage.Mode == StorageBind {
		s = append(s, Service{Name: "data-init", OneShot: true})
	}
	s = append(s,
		Service{Name: "mongodb"},
		Service{Name: "mongodb-init", OneShot: true},
		Service{Name: "opensearch", HashVar: HashVarOpenSearch},
		Service{Name: "openarch-app", HashVar: HashVarApp},
	)
	if c.Has(WithDashboards) {
		s = append(s, Service{Name: "opensearch-dashboards", HashVar: HashVarDashboards})
	}
	if c.TLSEnabled() {
		s = append(s, Service{Name: "proxy", HashVar: HashVarProxy})
	}
	return s
}

// ServiceNames returns the names of Services, in the same order.
func (c Config) ServiceNames() []string {
	var names []string
	for _, s := range c.Services() {
		names = append(names, s.Name)
	}
	return names
}
