// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectDriveWear(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.drives {
		wg.Add(1)
		go func(id string) {
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

			mu.Lock()
			mx.Drive[id] = driveMetrics{
				EnduranceRemaining: last.PercentEnduranceRemaining,
			}
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
