package example

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"

// Config holds the configuration for the example collector
type Config struct {
	// Embed framework configuration
	framework.Config `yaml:",inline" json:",inline"`

	// Connection endpoint (for demonstration)
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint"`

	// Connection timeout in seconds
	ConnectTimeout int `yaml:"connect_timeout,omitempty" json:"connect_timeout"`

	// Whether to collect item metrics
	CollectItems bool `yaml:"collect_items" json:"collect_items"`

	// Maximum number of items to collect
	MaxItems int `yaml:"max_items,omitempty" json:"max_items"`
}
