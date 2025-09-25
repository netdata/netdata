package mp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const defaultTimeout = 10 * time.Second

// Config captures user configuration for the WebSphere MicroProfile module.
// Comments are consumed by the documentation generator, so keep them precise.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`
	web.HTTPConfig   `yaml:",inline" json:",inline"`

	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	// CellName appends the Liberty cell label to every exported time-series.
	CellName string `yaml:"cell_name,omitempty" json:"cell_name"`
	// NodeName appends the Liberty node label to every exported time-series.
	NodeName string `yaml:"node_name,omitempty" json:"node_name"`
	// ServerName appends the Liberty server label to every exported time-series.
	ServerName string `yaml:"server_name,omitempty" json:"server_name"`

	// MetricsEndpoint overrides the metrics path relative to the base URL (accepts absolute URLs as well).
	MetricsEndpoint string `yaml:"metrics_endpoint,omitempty" json:"metrics_endpoint"`

	// CollectJVMMetrics toggles JVM/base scope metrics scraped from the MicroProfile endpoint.
	CollectJVMMetrics bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
	// CollectRESTMetrics toggles per-endpoint REST/JAX-RS metrics (may introduce cardinality).
	CollectRESTMetrics bool `yaml:"collect_rest_metrics" json:"collect_rest_metrics"`

	// MaxRESTEndpoints limits how many REST endpoints are exported when REST metrics are enabled (0 disables the limit).
	MaxRESTEndpoints int `yaml:"max_rest_endpoints,omitempty" json:"max_rest_endpoints"`
	// CollectRESTMatching filters REST endpoints using glob-style patterns (supports `*`, `?`, `!` prefixes).
	CollectRESTMatching string `yaml:"collect_rest_matching,omitempty" json:"collect_rest_matching"`
}
