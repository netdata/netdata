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

func (d *DnsmasqDHCP) collect() (map[string]int64, error) {
	now := time.Now()
	var updated bool

	if now.Sub(d.parseConfigTime) > d.parseConfigEvery {
		d.parseConfigTime = now

		dhcpRanges, dhcpHosts := d.parseDnsmasqDHCPConfiguration()
		d.dhcpRanges, d.dhcpHosts = dhcpRanges, dhcpHosts
		updated = d.updateCharts()

		d.collectV4V6Stats()
	}

	f, err := os.Open(d.LeasesPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if !updated {
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}

		if d.leasesModTime.Equal(fi.ModTime()) {
			d.Debug("lease database file modification time has not changed, old data is returned")
			return d.mx, nil
		}

		d.Debug("leases db file modification time has changed, reading it")
		d.leasesModTime = fi.ModTime()
	}

	leases := findLeases(f)
	d.collectRangesStats(leases)

	return d.mx, nil
}

func (d *DnsmasqDHCP) collectV4V6Stats() {
	d.mx["ipv4_dhcp_ranges"], d.mx["ipv6_dhcp_ranges"] = 0, 0
	for _, r := range d.dhcpRanges {
		if r.Family() == iprange.V6Family {
			d.mx["ipv6_dhcp_ranges"]++
		} else {
			d.mx["ipv4_dhcp_ranges"]++
		}
	}

	d.mx["ipv4_dhcp_hosts"], d.mx["ipv6_dhcp_hosts"] = 0, 0
	for _, ip := range d.dhcpHosts {
		if ip.To4() == nil {
			d.mx["ipv6_dhcp_hosts"]++
		} else {
			d.mx["ipv4_dhcp_hosts"]++
		}
	}
}

func (d *DnsmasqDHCP) collectRangesStats(leases []net.IP) {
	for _, r := range d.dhcpRanges {
		d.mx["dhcp_range_"+r.String()+"_allocated_leases"] = 0
		d.mx["dhcp_range_"+r.String()+"_utilization"] = 0
	}

	for _, ip := range leases {
		for _, r := range d.dhcpRanges {
			if r.Contains(ip) {
				d.mx["dhcp_range_"+r.String()+"_allocated_leases"]++
				break
			}
		}
	}

	for _, ip := range d.dhcpHosts {
		for _, r := range d.dhcpRanges {
			if r.Contains(ip) {
				d.mx["dhcp_range_"+r.String()+"_allocated_leases"]++
				break
			}
		}
	}

	for _, r := range d.dhcpRanges {
		name := "dhcp_range_" + r.String() + "_allocated_leases"
		numOfIps, ok := d.mx[name]
		if !ok {
			d.mx[name] = 0
		}
		d.mx["dhcp_range_"+r.String()+"_utilization"] = int64(math.Round(calcPercent(numOfIps, r.Size())))
	}
}

func (d *DnsmasqDHCP) updateCharts() bool {
	var updated bool
	seen := make(map[string]bool)
	for _, r := range d.dhcpRanges {
		seen[r.String()] = true
		if !d.cacheDHCPRanges[r.String()] {
			d.cacheDHCPRanges[r.String()] = true
			d.addDHCPRangeCharts(r.String())
			updated = true
		}
	}

	for v := range d.cacheDHCPRanges {
		if !seen[v] {
			delete(d.cacheDHCPRanges, v)
			d.removeDHCPRangeCharts(v)
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
