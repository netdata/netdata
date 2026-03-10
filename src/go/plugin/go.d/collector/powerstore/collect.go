// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"
)

const discoveryEvery = 5

func (c *Collector) collect() (map[string]int64, error) {
	c.runs++
	if !c.lastDiscoveryOK || c.runs%discoveryEvery == 0 {
		if err := c.discovery(); err != nil {
			return nil, err
		}
	}

	mx := metrics{
		Appliance:  make(map[string]applianceMetrics),
		Volume:     make(map[string]volumeMetrics),
		Node:       make(map[string]nodeMetrics),
		FcPort:     make(map[string]fcPortMetrics),
		EthPort:    make(map[string]ethPortMetrics),
		FileSystem: make(map[string]fileSystemMetrics),
		Drive:      make(map[string]driveMetrics),
	}

	c.collectClusterSpace(&mx)
	c.collectAppliances(&mx)
	c.collectVolumes(&mx)
	c.collectNodes(&mx)
	c.collectFcPorts(&mx)
	c.collectEthPorts(&mx)
	c.collectFileSystems(&mx)
	c.collectHardwareHealth(&mx)
	c.collectAlerts(&mx)
	c.collectDriveWear(&mx)
	c.collectNASStatus(&mx)
	c.collectReplication(&mx)

	c.updateCharts()
	return stm.ToMap(mx), nil
}

func (c *Collector) discovery() error {
	start := time.Now()
	c.Debugf("starting discovery")

	clusters, err := c.client.Clusters()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.clusters = clusters

	appliances, err := c.client.Appliances()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.appliances = make(map[string]client.Appliance, len(appliances))
	for _, a := range appliances {
		c.discovered.appliances[a.ID] = a
	}

	volumes, err := c.client.Volumes()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.volumes = make(map[string]client.Volume, len(volumes))
	for _, v := range volumes {
		if c.volumeMatches(v.Name) {
			c.discovered.volumes[v.ID] = v
		}
	}

	allHW, err := c.client.AllHardware()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.hardware = allHW
	c.discovered.nodes = make(map[string]client.Node)
	c.discovered.drives = make(map[string]client.Hardware)
	for _, h := range allHW {
		switch h.Type {
		case "Node":
			c.discovered.nodes[h.ID] = client.Node{ID: h.ID, Name: h.Name, ApplianceID: h.ApplianceID}
		case "Drive":
			c.discovered.drives[h.ID] = h
		}
	}

	fcPorts, err := c.client.FcPorts()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.fcPorts = make(map[string]client.FcPort, len(fcPorts))
	for _, p := range fcPorts {
		c.discovered.fcPorts[p.ID] = p
	}

	ethPorts, err := c.client.EthPorts()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.ethPorts = make(map[string]client.EthPort, len(ethPorts))
	for _, p := range ethPorts {
		c.discovered.ethPorts[p.ID] = p
	}

	fileSystems, err := c.client.FileSystems()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.fileSystems = make(map[string]client.FileSystem, len(fileSystems))
	for _, fs := range fileSystems {
		c.discovered.fileSystems[fs.ID] = fs
	}

	nas, err := c.client.NASServers()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.discovered.nasServers = make(map[string]client.NAS, len(nas))
	for _, n := range nas {
		c.discovered.nasServers[n.ID] = n
	}

	c.Debugf("discovery: %d clusters, %d appliances, %d volumes, %d nodes, %d FC ports, %d ETH ports, %d file systems, %d NAS servers, %d drives (took %s)",
		len(clusters), len(appliances), len(c.discovered.volumes), len(c.discovered.nodes),
		len(fcPorts), len(ethPorts), len(fileSystems), len(nas), len(c.discovered.drives),
		time.Since(start))

	c.lastDiscoveryOK = true
	return nil
}
