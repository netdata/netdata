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

	// Set the absolute value metric using type-safe API
	contexts.Test.TestAbsolute.Set(c.State, contexts.EmptyLabels{}, contexts.TestTestAbsoluteValues{
		Value: value,
	})

	// Get the counter value from the protocol (server maintains the counter)
	counterValue, err := c.client.GetCounterValue()
	if err != nil {
		return err
	}

	// Track the incremental change locally
	if counterValue > c.counter {
		// Set the incremental metric with the counter value using type-safe API
		// The framework will calculate the rate because algo is "incremental"
		contexts.Test.TestIncremental.Set(c.State, contexts.EmptyLabels{}, contexts.TestTestIncrementalValues{
			Counter: counterValue,
		})
		c.counter = counterValue
	}

	c.Debugf("Protocol returned value: %d, counter: %d", value, counterValue)

	return nil
}
