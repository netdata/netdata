// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import (
	"bufio"
	"io"
	"math"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

func (c *Collector) collect() (map[string]int64, error) {
	now := time.Now()
	var updated bool

	if now.Sub(c.parseConfigTime) > c.parseConfigEvery {
		c.parseConfigTime = now

		dhcpRanges, dhcpHosts := c.parseDnsmasqDHCPConfiguration()
		c.dhcpRanges, c.dhcpHosts = dhcpRanges, dhcpHosts
		updated = c.updateCharts()

		c.collectV4V6Stats()
	}

	f, err := os.Open(c.LeasesPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if !updated {
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}

		if c.leasesModTime.Equal(fi.ModTime()) {
			c.Debug("lease database file modification time has not changed, old data is returned")
			return c.mx, nil
		}

		c.Debug("leases db file modification time has changed, reading it")
		c.leasesModTime = fi.ModTime()
	}

	leases := findLeases(f)
	c.collectRangesStats(leases)

	return c.mx, nil
}

func (c *Collector) collectV4V6Stats() {
	c.mx["ipv4_dhcp_ranges"], c.mx["ipv6_dhcp_ranges"] = 0, 0
	for _, r := range c.dhcpRanges {
		if r.Family() == iprange.V6Family {
			c.mx["ipv6_dhcp_ranges"]++
		} else {
			c.mx["ipv4_dhcp_ranges"]++
		}
	}

	c.mx["ipv4_dhcp_hosts"], c.mx["ipv6_dhcp_hosts"] = 0, 0
	for _, ip := range c.dhcpHosts {
		if ip.To4() == nil {
			c.mx["ipv6_dhcp_hosts"]++
		} else {
			c.mx["ipv4_dhcp_hosts"]++
		}
	}
}

func (c *Collector) collectRangesStats(leases []net.IP) {
	for _, r := range c.dhcpRanges {
		c.mx["dhcp_range_"+r.String()+"_allocated_leases"] = 0
		c.mx["dhcp_range_"+r.String()+"_utilization"] = 0
	}

	for _, ip := range leases {
		for _, r := range c.dhcpRanges {
			if r.Contains(ip) {
				c.mx["dhcp_range_"+r.String()+"_allocated_leases"]++
				break
			}
		}
	}

	for _, ip := range c.dhcpHosts {
		for _, r := range c.dhcpRanges {
			if r.Contains(ip) {
				c.mx["dhcp_range_"+r.String()+"_allocated_leases"]++
				break
			}
		}
	}

	for _, r := range c.dhcpRanges {
		name := "dhcp_range_" + r.String() + "_allocated_leases"
		numOfIps, ok := c.mx[name]
		if !ok {
			c.mx[name] = 0
		}
		c.mx["dhcp_range_"+r.String()+"_utilization"] = int64(math.Round(calcPercent(numOfIps, r.Size())))
	}
}

func (c *Collector) updateCharts() bool {
	var updated bool
	seen := make(map[string]bool)
	for _, r := range c.dhcpRanges {
		seen[r.String()] = true
		if !c.cacheDHCPRanges[r.String()] {
			c.cacheDHCPRanges[r.String()] = true
			c.addDHCPRangeCharts(r.String())
			updated = true
		}
	}

	for v := range c.cacheDHCPRanges {
		if !seen[v] {
			delete(c.cacheDHCPRanges, v)
			c.removeDHCPRangeCharts(v)
			updated = true
		}
	}
	return updated
}

func findLeases(r io.Reader) []net.IP {
	/*
		1560300536 08:00:27:61:3c:ee 2.2.2.3 debian8 *
		duid 00:01:00:01:24:90:cf:5b:08:00:27:61:2e:2c
		1560300414 660684014 1234::20b * 00:01:00:01:24:90:cf:a3:08:00:27:61:3c:ee
	*/
	var ips []net.IP
	s := bufio.NewScanner(r)

	for s.Scan() {
		parts := strings.Fields(s.Text())
		if len(parts) != 5 {
			continue
		}

		ip := net.ParseIP(parts[2])
		if ip == nil {
			continue
		}
		ips = append(ips, ip)
	}

	return ips
}

func calcPercent(ips int64, hosts *big.Int) float64 {
	h := hosts.Int64()
	if ips == 0 || h == 0 || !hosts.IsInt64() {
		return 0
	}
	return float64(ips) * 100 / float64(h)
}
