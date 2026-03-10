// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectReplication(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.appliances {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			cm, err := c.client.CopyMetricsByAppliance(id)
			if err != nil {
				c.Warningf("error collecting appliance %s copy metrics: %v", id, err)
				return
			}
			if len(cm) == 0 {
				return
			}

			last := cm[len(cm)-1]

			mu.Lock()
			if last.DataRemaining != nil {
				mx.Copy.DataRemaining += *last.DataRemaining
			}
			if last.DataTransferred != nil {
				mx.Copy.DataTransferred += *last.DataTransferred
			}
			mx.Copy.TransferRate += last.TransferRate
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
