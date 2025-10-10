package mp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const defaultTimeout = 10 * time.Second

// Config captures user configuration for the WebSphere MicroProfile module.
// Comments are consumed by the documentation generator, so keep them precise.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`
	web.HTTPConfig   `yaml:",inline" json:",inline"`

	Vnode string `yaml:"vnode,omitempty" json:"vnode" ui:"group:Connection"`

	// CellName appends the Liberty cell label to every exported time-series.
	CellName string `yaml:"cell_name,omitempty" json:"cell_name" ui:"group:Identity"`
	// NodeName appends the Liberty node label to every exported time-series.
	NodeName string `yaml:"node_name,omitempty" json:"node_name" ui:"group:Identity"`
	// ServerName appends the Liberty server label to every exported time-series.
	ServerName string `yaml:"server_name,omitempty" json:"server_name" ui:"group:Identity"`

	// MetricsEndpoint overrides the metrics path relative to the base URL (accepts absolute URLs as well).
	MetricsEndpoint string `yaml:"metrics_endpoint,omitempty" json:"metrics_endpoint" ui:"group:Connection"`

	// CollectJVMMetrics toggles JVM/base scope metrics scraped from the MicroProfile endpoint.
	CollectJVMMetrics framework.AutoBool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics" ui:"group:Other Metrics"`
	// CollectRESTMetrics toggles per-endpoint REST/JAX-RS metrics (may introduce cardinality).
	CollectRESTMetrics framework.AutoBool `yaml:"collect_rest_metrics" json:"collect_rest_metrics" ui:"group:REST Endpoints"`

	// MaxRESTEndpoints limits how many REST endpoints are exported when REST metrics are enabled (0 disables the limit).
	MaxRESTEndpoints int `yaml:"max_rest_endpoints,omitempty" json:"max_rest_endpoints" ui:"group:REST Endpoints"`
	// CollectRESTMatching filters REST endpoints using glob-style patterns (supports `*`, `?`, `!` prefixes).
	CollectRESTMatching string `yaml:"collect_rest_matching,omitempty" json:"collect_rest_matching" ui:"group:REST Endpoints"`
}
