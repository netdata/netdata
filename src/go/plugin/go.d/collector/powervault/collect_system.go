// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

// collectSystemHealth reports overall system health.
// Uses cached discovery data — no API calls.
// health-numeric: 0=OK, 1=Degraded, 2=Fault, 3=Unknown.
func (c *Collector) collectSystemHealth() {
	if len(c.discovered.system) == 0 {
		return
	}
	c.mx.system.health.Observe(float64(c.discovered.system[0].HealthNumeric))
}
