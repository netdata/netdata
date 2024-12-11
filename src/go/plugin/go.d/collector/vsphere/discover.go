// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

func (c *Collector) goDiscovery() {
	if c.discoveryTask != nil {
		c.discoveryTask.stop()
	}
	c.Infof("starting discovery process, will do discovery every %s", c.DiscoveryInterval)

	job := func() {
		err := c.discoverOnce()
		if err != nil {
			c.Errorf("error on discovering : %v", err)
		}
	}
	c.discoveryTask = newTask(job, c.DiscoveryInterval.Duration())
}

func (c *Collector) discoverOnce() error {
	res, err := c.Discover()
	if err != nil {
		return err
	}

	c.collectionLock.Lock()
	c.resources = res
	c.collectionLock.Unlock()

	return nil
}
