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
		true,                            // collectMetrics (always)
		c.Config.MaxListeners,           // maxListeners (0 = no limit)
		c.Config.ListenerSelector,       // selector pattern
		c.Config.CollectSystemListeners, // collectSystem
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
		result.Stats.Discovery.AvailableItems-result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Listeners),
		failed)

	// Process collected listener metrics
	for _, lm := range result.Listeners {
		// Create labels with port and IP address
		ipAddress := lm.IPAddress
		if ipAddress == "" {
			ipAddress = "*" // Default for listeners bound to all interfaces
		}

		labels := contexts.ListenerLabels{
			Listener:   lm.Name,
			Port:       fmt.Sprintf("%d", lm.Port),
			Ip_address: ipAddress,
		}

		// Set status - only one status can be active at a time
		statusValues := contexts.ListenerStatusValues{
			Stopped:  0,
			Starting: 0,
			Running:  0,
			Stopping: 0,
			Retrying: 0,
		}

		switch lm.Status {
		case pcf.ListenerStatusStopped:
			statusValues.Stopped = 1
		case pcf.ListenerStatusStarting:
			statusValues.Starting = 1
		case pcf.ListenerStatusRunning:
			statusValues.Running = 1
		case pcf.ListenerStatusStopping:
			statusValues.Stopping = 1
		case pcf.ListenerStatusRetrying:
			statusValues.Retrying = 1
		default:
			// Unknown status - treat as stopped
			statusValues.Stopped = 1
			c.Debugf("Unknown listener status %d for %s", lm.Status, lm.Name)
		}

		contexts.Listener.Status.Set(c.State, labels, statusValues)

		// Set backlog if collected
		if lm.Backlog.IsCollected() {
			contexts.Listener.Backlog.Set(c.State, labels, contexts.ListenerBacklogValues{
				Backlog: lm.Backlog.Int64(),
			})
		}

		// Set uptime if running
		if lm.Status == pcf.ListenerStatusRunning && lm.Uptime > 0 {
			contexts.Listener.Uptime.Set(c.State, labels, contexts.ListenerUptimeValues{
				Uptime: lm.Uptime,
			})
		}
	}

	return nil
}
