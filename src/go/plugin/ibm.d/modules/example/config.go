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

// SetDefaults sets default configuration values
func (c *Config) SetDefaults() {
	// Framework defaults
	if c.ObsoletionIterations == 0 {
		c.ObsoletionIterations = 60
	}
	if c.UpdateEvery == 0 {
		c.UpdateEvery = 1
	}
	
	// Module-specific defaults
	if c.Endpoint == "" {
		c.Endpoint = "dummy://localhost"
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5
	}
	// Default to collecting items
	c.CollectItems = true
	
	// Default max items
	if c.MaxItems == 0 {
		c.MaxItems = 10
	}
}