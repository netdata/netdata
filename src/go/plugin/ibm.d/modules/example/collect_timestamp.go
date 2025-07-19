package example

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example/contexts"
)

// collectTimestampMetrics collects the timestamp-based metrics
func (c *Collector) collectTimestampMetrics() error {
	// Make a protocol call to get the timestamp value
	value, err := c.client.GetTimestampValue()
	if err != nil {
		return err
	}
	
	// Set the absolute value metric
	c.State.SetGlobal(contexts.Test.TestAbsolute, map[string]int64{
		"value": value,
	})
	
	// Get the counter value from the protocol (server maintains the counter)
	counterValue, err := c.client.GetCounterValue()
	if err != nil {
		return err
	}
	
	// Track the incremental change locally
	if counterValue > c.counter {
		// Set the incremental metric with the counter value
		// The framework will calculate the rate because algo is "incremental"
		c.State.SetGlobal(contexts.Test.TestIncremental, map[string]int64{
			"counter": counterValue,
		})
		c.counter = counterValue
	}
	
	c.Debugf("Protocol returned value: %d, counter: %d", value, counterValue)
	
	return nil
}