// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

func (vs *VSphere) goDiscovery() {
	if vs.discoveryTask != nil {
		vs.discoveryTask.stop()
	}
	vs.Infof("starting discovery process, will do discovery every %s", vs.DiscoveryInterval)

	job := func() {
		err := vs.discoverOnce()
		if err != nil {
			vs.Errorf("error on discovering : %v", err)
		}
	}
	vs.discoveryTask = newTask(job, vs.DiscoveryInterval.Duration())
}

func (vs *VSphere) discoverOnce() error {
	res, err := vs.Discover()
	if err != nil {
		return err
	}

	vs.collectionLock.Lock()
	vs.resources = res
	vs.collectionLock.Unlock()

	return nil
}
