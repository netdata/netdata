package example

import (
	"context"
	"fmt"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example/dummy"
)

// Init initializes the collector
func (c *Collector) Init(ctx context.Context) error {
	// Set this collector as the implementation
	c.SetImpl(c)
	
	// Set defaults
	c.config.SetDefaults()
	
	// Copy framework configuration from module config to framework
	c.Config.ObsoletionIterations = c.config.ObsoletionIterations
	c.Config.UpdateEvery = c.config.UpdateEvery
	c.Config.CollectionGroups = c.config.CollectionGroups
	
	// Initialize protocol client
	c.client = dummy.NewClient()
	
	// Initialize maps
	c.itemData = make(map[int]*ItemData)
	
	// Register all contexts from generated code BEFORE base init
	c.RegisterContexts(contexts.GetAllContexts()...)
	
	// Initialize base collector (which will create charts)
	if err := c.Collector.Init(ctx); err != nil {
		return err
	}
	return nil
}

// Check tests connectivity
func (c *Collector) Check(ctx context.Context) error {
	// Test protocol connectivity
	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	
	// Test fetching a value
	if _, err := c.client.GetTimestampValue(); err != nil {
		c.client.Disconnect()
		return fmt.Errorf("failed to fetch test value: %v", err)
	}
	
	// Get version information and set as global labels
	version, edition, err := c.client.GetVersion()
	if err == nil {
		c.SetGlobalLabel("app_version", version)
		c.SetGlobalLabel("app_edition", edition)
		c.SetGlobalLabel("endpoint", c.config.Endpoint)
		c.Infof("Connected to version %s (%s edition) at %s", version, edition, c.config.Endpoint)
	}
	
	c.Infof("Example collector check successful")
	return nil
}

// Cleanup closes connections
func (c *Collector) Cleanup(ctx context.Context) {
	// Disconnect from protocol
	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect()
	}
}