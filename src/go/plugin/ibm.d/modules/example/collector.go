package example

import (
	"fmt"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example/dummy"
)

// ItemData holds persistent data for an item instance
type ItemData struct {
	Counter int64
}

// Collector is the example collector demonstrating the new framework
type Collector struct {
	framework.Collector
	
	// Configuration
	config Config
	
	// Protocol client
	client *dummy.Client
	
	// Job-level persistent data
	counter int64 // Local counter that tracks incremental values
	
	// Instance-level persistent data
	itemData map[int]*ItemData // Per-instance persistent data
}

// CollectOnce implements the actual collection logic for a single iteration
func (c *Collector) CollectOnce() error {
	// Ensure we're connected
	if !c.client.IsConnected() {
		if err := c.client.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
	}
	
	// Update global labels - the protocol caches version info and refreshes on reconnect
	version, edition, err := c.client.GetVersion()
	if err == nil {
		c.SetGlobalLabel("app_version", version)
		c.SetGlobalLabel("app_edition", edition)
		c.SetGlobalLabel("endpoint", c.config.Endpoint)
	}
	
	// Collect timestamp value
	if err := c.collectTimestampMetrics(); err != nil {
		return fmt.Errorf("failed to collect timestamp metrics: %v", err)
	}
	
	// Collect item metrics if enabled
	if c.config.CollectItems {
		if err := c.collectItemMetrics(); err != nil {
			return fmt.Errorf("failed to collect item metrics: %v", err)
		}
	}
	
	return nil
}


