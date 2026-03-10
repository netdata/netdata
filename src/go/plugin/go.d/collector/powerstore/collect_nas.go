// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectNASStatus(mx *metrics) {
	for _, nas := range c.discovered.naServers {
		switch nas.OperationalStatus {
		case "Started":
			mx.NAS.Started++
		case "Stopped":
			mx.NAS.Stopped++
		case "Degraded":
			mx.NAS.Degraded++
		default:
			mx.NAS.Unknown++
		}
	}
}
