// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectDriveWear(mx *metrics) {
	for id := range c.discovered.drives {
		wm, err := c.client.WearMetricsByDrive(id)
		if err != nil {
			c.Warningf("error collecting drive %s wear metrics: %v", id, err)
			continue
		}
		if len(wm) == 0 {
			continue
		}

		last := wm[len(wm)-1]
		mx.Drive[id] = driveMetrics{
			EnduranceRemaining: last.PercentEnduranceRemaining,
		}
	}
}
