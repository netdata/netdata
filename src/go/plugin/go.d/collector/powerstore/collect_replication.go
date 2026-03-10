// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectReplication(mx *metrics) {
	for id := range c.discovered.appliances {
		cm, err := c.client.CopyMetricsByAppliance(id)
		if err != nil {
			c.Warningf("error collecting appliance %s copy metrics: %v", id, err)
			continue
		}
		if len(cm) == 0 {
			continue
		}

		last := cm[len(cm)-1]
		if last.DataRemaining != nil {
			mx.Copy.DataRemaining += *last.DataRemaining
		}
		if last.DataTransferred != nil {
			mx.Copy.DataTransferred += *last.DataTransferred
		}
		mx.Copy.TransferRate += last.TransferRate
	}
}
