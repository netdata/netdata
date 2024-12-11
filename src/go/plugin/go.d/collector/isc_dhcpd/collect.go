// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"os"
)

/*
dhcpd.leases db (file), see details: https://kb.isc.org/docs/en/isc-dhcp-44-manual-pages-dhcpdleases#dhcpdleases

Every time a lease is acquired, renewed or released, its new value is recorded at the end of the lease file.
So if more than one declaration appears for a given lease, the last one in the file is the current one.

In order to prevent the lease database from growing without bound, the file is rewritten from time to time.
First, a temporary lease database is created and all known leases are dumped to it.
Then, the old lease database is renamed DBDIR/dhcpd.leases~.
Finally, the newly written lease database is moved into place.

In order to process both DHCPv4 and DHCPv6 messages you will need to run two separate instances of the dhcpd process.
Each of these instances will need its own lease file.
*/

func (c *Collector) collect() (map[string]int64, error) {
	fi, err := os.Stat(c.LeasesPath)
	if err != nil {
		return nil, err
	}

	if c.leasesModTime.Equal(fi.ModTime()) {
		c.Debugf("leases file is not modified, returning cached metrics ('%s')", c.LeasesPath)
		return c.collected, nil
	}

	c.leasesModTime = fi.ModTime()

	leases, err := parseDHCPdLeasesFile(c.LeasesPath)
	if err != nil {
		return nil, err
	}

	activeLeases := removeInactiveLeases(leases)
	c.Debugf("found total/active %d/%d leases ('%s')", len(leases), len(activeLeases), c.LeasesPath)

	for _, pool := range c.pools {
		collectPool(c.collected, pool, activeLeases)
	}
	c.collected["active_leases_total"] = int64(len(activeLeases))

	return c.collected, nil
}

const precision = 100

func collectPool(collected map[string]int64, pool ipPool, leases []leaseEntry) {
	n := calcPoolActiveLeases(pool, leases)
	collected["dhcp_pool_"+pool.name+"_active_leases"] = n
	collected["dhcp_pool_"+pool.name+"_utilization"] = int64(calcPoolUtilizationPercentage(pool, n) * precision)
}

func calcPoolActiveLeases(pool ipPool, leases []leaseEntry) (num int64) {
	for _, l := range leases {
		if pool.addresses.Contains(l.ip) {
			num++
		}
	}
	return num
}

func calcPoolUtilizationPercentage(pool ipPool, leases int64) float64 {
	size := pool.addresses.Size()
	if leases == 0 || !size.IsInt64() {
		return 0
	}
	if size.Int64() == 0 {
		return 100
	}
	return float64(leases) / float64(size.Int64()) * 100
}

func removeInactiveLeases(leases []leaseEntry) (active []leaseEntry) {
	active = leases[:0]
	for _, l := range leases {
		if l.bindingState == "active" {
			active = append(active, l)
		}
	}
	return active
}
