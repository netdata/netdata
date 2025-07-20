package mq

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"

func (c *Collector) collectListenerMetrics() error {
	listeners, err := c.client.GetListeners()
	if err != nil {
		return err
	}

	for _, listener := range listeners {
		labels := contexts.ListenerLabels{
			Listener: listener.Name,
		}

		// Collect listener status - convert enum to individual dimensions
		var running, stopped int64
		switch listener.Status {
		case 1: // Running
			running = 1
			stopped = 0
		case 0: // Stopped
			running = 0
			stopped = 1
		default:
			// Unknown status - treat as stopped
			running = 0
			stopped = 1
		}

		contexts.Listener.Status.Set(c.State, labels, contexts.ListenerStatusValues{
			Running: running,
			Stopped: stopped,
		})

		// Collect listener port if available
		if listener.Port > 0 {
			contexts.Listener.Port.Set(c.State, labels, contexts.ListenerPortValues{
				Port: listener.Port,
			})
		}
	}

	return nil
}