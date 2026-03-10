// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectNASStatus() {
	var started, stopped, degraded, unknown float64

	for _, nas := range c.discovered.nasServers {
		switch nas.OperationalStatus {
		case "Started":
			started++
		case "Stopped":
			stopped++
		case "Degraded":
			degraded++
		default:
			unknown++
		}
	}

	c.mx.nas.started.Observe(started)
	c.mx.nas.stopped.Observe(stopped)
	c.mx.nas.degraded.Observe(degraded)
	c.mx.nas.unknown.Observe(unknown)
}
