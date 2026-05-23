// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

// collectDriveMetrics reports per-drive temperature, power-on hours, and SSD life.
// Uses cached discovery data — no API calls.
func (c *Collector) collectDriveMetrics() {
	for _, d := range c.discovered.drives {
		id := d.Location
		c.mx.drive.temperature.WithLabelValues(id).Observe(float64(d.TempNumeric))
		c.mx.drive.powerOnHours.WithLabelValues(id).Observe(float64(d.PowerOnHours))

		// SSD life left: 255 means N/A (HDD). Only report for SSDs.
		if d.SSDLifeLeft != 255 {
			c.mx.drive.ssdLifeLeft.WithLabelValues(id).Observe(float64(d.SSDLifeLeft))
		}
	}
}
