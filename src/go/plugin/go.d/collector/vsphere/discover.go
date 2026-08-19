// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import "fmt"

func (c *Collector) goDiscovery() {
	c.stopDiscoveryTask(false)
	c.Infof("starting discovery process, will do discovery every %s", c.DiscoveryInterval)

	job := func() {
		err := c.discoverOnce()
		if err != nil {
			c.Limit(logKeyDiscoveryError, 1, recurringLogEvery).
				Errorf("periodic vSphere discovery failed: %v", err)
		}
	}
	c.discoveryTask = newTask(job, c.DiscoveryInterval.Duration())
}

func (c *Collector) discoverOnce() error {
	res, err := c.Discover()
	if err != nil {
		return fmt.Errorf("discover vSphere resources through configured discoverer: %w", err)
	}

	c.collectionLock.Lock()
	c.resources = res
	c.collectionLock.Unlock()

	return nil
}
