// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectDriveWear() {
	var wg sync.WaitGroup

	for id, drv := range c.discovered.drives {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			wm, err := c.client.WearMetricsByDrive(id)
			if err != nil {
				c.Warningf("error collecting drive %s wear metrics: %v", id, err)
				return
			}
			if len(wm) == 0 {
				return
			}

			last := wm[len(wm)-1]
			c.mx.drive.enduranceRemaining.WithLabelValues(name).Observe(float64(last.PercentEnduranceRemaining))
		}(id, drv.Name)
	}

	wg.Wait()
}
