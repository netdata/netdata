package mq

import (
	"fmt"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func (c *Collector) collectListenerMetrics() error {
	c.Debugf("Collecting listeners with selector '%s', system: %v", 
		c.Config.ListenerSelector, c.Config.CollectSystemListeners)
	
	// Use new GetListeners with transparency
	result, err := c.client.GetListeners(
		true,                             // collectMetrics (always)
		c.Config.MaxListeners,            // maxListeners (0 = no limit)
		c.Config.ListenerSelector,        // selector pattern
		c.Config.CollectSystemListeners,  // collectSystem
	)
	if err != nil {
		return fmt.Errorf("failed to collect listener metrics: %w", err)
	}
	
	// Check discovery success
	if !result.Stats.Discovery.Success {
		c.Errorf("Listener discovery failed completely")
		return fmt.Errorf("listener discovery failed")
	}
	
	// Map transparency counters to user-facing semantics
	monitored := int64(0)
	if result.Stats.Metrics != nil {
		monitored = result.Stats.Metrics.OkItems
	}
	
	failed := result.Stats.Discovery.UnparsedItems
	if result.Stats.Metrics != nil {
		failed += result.Stats.Metrics.FailedItems
	}
	
	// Update overview metrics with correct semantics  
	c.setListenerOverviewMetrics(
		monitored,                             // monitored (successfully enriched)
		result.Stats.Discovery.ExcludedItems,  // excluded (filtered by user)
		result.Stats.Discovery.InvisibleItems, // invisible (discovery errors)
		failed,                                // failed (unparsed + enrichment failures)
	)
	
	// Log collection summary
	c.Debugf("Listener collection complete - discovered:%d visible:%d included:%d collected:%d failed:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Listeners),
		failed)
	
	// Process collected listener metrics
	for _, listener := range result.Listeners {
		labels := contexts.ListenerLabels{
			Listener: listener.Name,
		}

		// Collect listener status - convert enum to individual dimensions
		statusValues := contexts.ListenerStatusValues{}
		
		switch listener.Status {
		case pcf.ListenerStatusRunning:
			statusValues.Running = 1
			statusValues.Stopped = 0
		case pcf.ListenerStatusStopped:
			statusValues.Running = 0
			statusValues.Stopped = 1
		default:
			// Unknown status - treat as stopped
			statusValues.Running = 0
			statusValues.Stopped = 1
		}

		contexts.Listener.Status.Set(c.State, labels, statusValues)

		// Collect listener port if available
		if listener.Port > 0 {
			contexts.Listener.Port.Set(c.State, labels, contexts.ListenerPortValues{
				Port: listener.Port,
			})
		}
	}
	
	return nil
}