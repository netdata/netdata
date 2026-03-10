// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

// collectHardwareHealth classifies hardware components by lifecycle state.
// Uses cached hardware data from discovery to avoid redundant API calls.
func (c *Collector) collectHardwareHealth(mx *metrics) {
	for _, h := range c.discovered.hardware {
		switch h.Type {
		case "Fan":
			classifyHardwareState(&mx.Hardware.Fan, h.LifecycleState)
		case "Power_Supply":
			classifyHardwareState(&mx.Hardware.PSU, h.LifecycleState)
		case "Drive":
			classifyHardwareState(&mx.Hardware.Drive, h.LifecycleState)
		case "Battery":
			classifyHardwareState(&mx.Hardware.Batt, h.LifecycleState)
		case "Node":
			classifyHardwareState(&mx.Hardware.Node, h.LifecycleState)
		}
	}
}

func classifyHardwareState(counts *hwStateCount, state string) {
	switch state {
	case "Healthy", "healthy":
		counts.OK++
	case "Degraded", "degraded":
		counts.Degraded++
	case "Failed", "failed", "Error", "error":
		counts.Failed++
	default:
		counts.Unknown++
	}
}
