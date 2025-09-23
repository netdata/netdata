package mp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const defaultTimeout = 10 * time.Second

// Config captures user configuration for the WebSphere MicroProfile module.
type Config struct {
    framework.Config `yaml:",inline" json:",inline"`
    web.HTTPConfig   `yaml:",inline" json:",inline"`

    Vnode string `yaml:"vnode,omitempty" json:"vnode"`

    // Identity labels
    CellName   string `yaml:"cell_name,omitempty" json:"cell_name"`
    NodeName   string `yaml:"node_name,omitempty" json:"node_name"`
    ServerName string `yaml:"server_name,omitempty" json:"server_name"`

    URL             string `yaml:"url" json:"url"`
    MetricsEndpoint string `yaml:"metrics_endpoint,omitempty" json:"metrics_endpoint"`

    CollectJVMMetrics  bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
    CollectRESTMetrics bool `yaml:"collect_rest_metrics" json:"collect_rest_metrics"`

    MaxRESTEndpoints    int    `yaml:"max_rest_endpoints,omitempty" json:"max_rest_endpoints"`
    CollectRESTMatching string `yaml:"collect_rest_matching,omitempty" json:"collect_rest_matching"`
}
