// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"
)

const discoveryEvery = 5

func (c *Collector) collect() error {
	c.runs++
	if !c.lastDiscoveryOK || c.runs%discoveryEvery == 0 {
		if err := c.discovery(); err != nil {
			return err
		}
	}

	// Collect metrics from API concurrently, bounded by semaphore.
	var wg sync.WaitGroup
	for _, fn := range []func(){
		c.collectClusterSpace,
		c.collectAppliances,
		c.collectVolumes,
		c.collectNodes,
		c.collectFcPorts,
		c.collectEthPorts,
		c.collectFileSystems,
		c.collectAlerts,
		c.collectDriveWear,
		c.collectReplication,
	} {
		wg.Add(1)
		go func(f func()) {
			defer wg.Done()
			f()
		}(fn)
	}
	wg.Wait()

	// These use cached discovery data, no API calls.
	c.collectHardwareHealth()
	c.collectNASStatus()

	return nil
}

func (c *Collector) discovery() error {
	start := time.Now()
	c.Debugf("starting discovery")

	// Build results in a temp struct; only swap on full success.
	var d discovered

	clusters, err := c.client.Clusters()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.clusters = clusters

	appliances, err := c.client.Appliances()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.appliances = make(map[string]client.Appliance, len(appliances))
	for _, a := range appliances {
		d.appliances[a.ID] = a
	}

	volumes, err := c.client.Volumes()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.volumes = make(map[string]client.Volume, len(volumes))
	for _, v := range volumes {
		if c.volumeMatches(v.Name) {
			d.volumes[v.ID] = v
		}
	}

	allHW, err := c.client.AllHardware()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.hardware = allHW
	d.nodes = make(map[string]client.Node)
	d.drives = make(map[string]client.Hardware)
	for _, h := range allHW {
		switch h.Type {
		case "Node":
			d.nodes[h.ID] = client.Node{ID: h.ID, Name: h.Name, ApplianceID: h.ApplianceID}
		case "Drive":
			d.drives[h.ID] = h
		}
	}

	fcPorts, err := c.client.FcPorts()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.fcPorts = make(map[string]client.FcPort, len(fcPorts))
	for _, p := range fcPorts {
		d.fcPorts[p.ID] = p
	}

	ethPorts, err := c.client.EthPorts()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.ethPorts = make(map[string]client.EthPort, len(ethPorts))
	for _, p := range ethPorts {
		d.ethPorts[p.ID] = p
	}

	fileSystems, err := c.client.FileSystems()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.fileSystems = make(map[string]client.FileSystem, len(fileSystems))
	for _, fs := range fileSystems {
		d.fileSystems[fs.ID] = fs
	}

	nas, err := c.client.NASServers()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.nasServers = make(map[string]client.NAS, len(nas))
	for _, n := range nas {
		d.nasServers[n.ID] = n
	}

	// All API calls succeeded — atomic swap.
	c.discovered = d
	c.lastDiscoveryOK = true

	c.Debugf("discovery: %d clusters, %d appliances, %d volumes, %d nodes, %d FC ports, %d ETH ports, %d file systems, %d NAS servers, %d drives (took %s)",
		len(d.clusters), len(d.appliances), len(d.volumes), len(d.nodes),
		len(d.fcPorts), len(d.ethPorts), len(d.fileSystems), len(d.nasServers), len(d.drives),
		time.Since(start))

	return nil
}
