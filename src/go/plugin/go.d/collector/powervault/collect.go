// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

import (
	"sync"
	"time"

	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powervault/client"
)

const discoveryEvery = 10

func (c *Collector) collect(ctx context.Context) error {
	c.runs++
	needsDiscovery := !c.lastDiscoveryOK || c.runs%discoveryEvery == 0
	wasOK := c.lastDiscoveryOK

	if needsDiscovery {
		if err := c.discovery(ctx); err != nil {
			// Initial discovery must succeed.
			if !wasOK {
				return err
			}
			// Refresh failure — continue with previous data.
			c.Warningf("discovery refresh failed, using previous data: %v", err)
		}
	}

	var wg sync.WaitGroup
	for _, fn := range []func(context.Context){
		c.collectControllerStats,
		c.collectVolumeStats,
		c.collectPortStats,
		c.collectPhyStats,
	} {
		wg.Add(1)
		go func(f func(context.Context)) {
			defer wg.Done()
			f(ctx)
		}(fn)
	}
	wg.Wait()

	// These use cached discovery data, no API calls.
	c.collectHardwareHealth()
	c.collectDriveMetrics()
	c.collectSensorMetrics()
	c.collectPoolCapacity()
	c.collectSystemHealth()

	return nil
}

func (c *Collector) discovery(ctx context.Context) error {
	start := time.Now()
	c.Debugf("starting discovery")

	var d discovered

	sys, err := c.client.System(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.system = sys

	controllers, err := c.client.Controllers(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.controllers = controllers

	drives, err := c.client.Drives(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.drives = drives

	fans, err := c.client.Fans(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.fans = fans

	psus, err := c.client.PowerSupplies(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.psus = psus

	sensors, err := c.client.Sensors(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.sensors = sensors

	frus, err := c.client.FRUs(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.frus = frus

	volumes, err := c.client.Volumes(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.volumes = make(map[string]client.Volume, len(volumes))
	for _, v := range volumes {
		if c.volumeMatches(v.VolumeName) {
			d.volumes[v.VolumeName] = v
		}
	}

	pools, err := c.client.Pools(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.pools = pools

	ports, err := c.client.Ports(ctx)
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	d.ports = ports

	// Atomic swap on full success.
	c.discovered = d
	c.lastDiscoveryOK = true

	c.Debugf("discovery: %d controllers, %d drives, %d fans, %d PSUs, %d sensors, %d FRUs, %d volumes, %d pools, %d ports (took %s)",
		len(d.controllers), len(d.drives), len(d.fans), len(d.psus),
		len(d.sensors), len(d.frus), len(d.volumes), len(d.pools), len(d.ports),
		time.Since(start))

	return nil
}
