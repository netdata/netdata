// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

const (
	precision = 1000000
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return nil, fmt.Errorf("creating NTP client: %v", err)
		}
		c.client = client
	}

	mx := make(map[string]int64)

	if err := c.collectInfo(mx); err != nil {
		return nil, err
	}

	if c.CollectPeers {
		if now := time.Now(); now.Sub(c.findPeersTime) > c.findPeersEvery {
			c.findPeersTime = now
			if err := c.findPeers(); err != nil {
				c.Warning(err)
			}
		}
		c.collectPeersInfo(mx)
	}

	return mx, nil
}

func (c *Collector) collectInfo(mx map[string]int64) error {
	info, err := c.client.systemInfo()
	if err != nil {
		return fmt.Errorf("error on querying system info: %v", err)
	}

	for k, v := range info {
		switch k {
		case
			"offset",
			"sys_jitter",
			"clk_jitter",
			"frequency",
			"clk_wander",
			"rootdelay",
			"rootdisp",
			"stratum",
			"tc",
			"mintc",
			"precision":
			if val, err := strconv.ParseFloat(v, 64); err == nil {
				mx[k] = int64(val * precision)
			}
		}
	}
	return nil
}

func (c *Collector) collectPeersInfo(mx map[string]int64) {
	for _, id := range c.peerIDs {
		info, err := c.client.peerInfo(id)
		if err != nil {
			c.Warningf("error on querying NTP peer info id='%d': %v", id, err)
			continue
		}

		addr, ok := info["srcadr"]
		if !ok {
			continue
		}

		for k, v := range info {
			switch k {
			case
				"offset",
				"delay",
				"dispersion",
				"jitter",
				"xleave",
				"rootdelay",
				"rootdisp",
				"stratum",
				"hmode",
				"pmode",
				"hpoll",
				"ppoll",
				"precision":
				if val, err := strconv.ParseFloat(v, 64); err == nil {
					mx["peer_"+addr+"_"+k] = int64(val * precision)
				}
			}
		}
	}
}

func (c *Collector) findPeers() error {
	c.peerIDs = c.peerIDs[:0]

	c.Debug("querying NTP peers")
	peers, err := c.client.peerIDs()
	if err != nil {
		return fmt.Errorf("querying NTP peers: %v", err)
	}

	c.Debugf("found %d NTP peers (ids: %v)", len(peers), peers)
	seen := make(map[string]bool)

	for _, id := range peers {
		info, err := c.client.peerInfo(id)
		if err != nil {
			c.Debugf("error on querying NTP peer info id='%d': %v", id, err)
			continue
		}

		addr, ok := info["srcadr"]
		if ip := net.ParseIP(addr); !ok || ip == nil || c.peerIPAddrFilter.Contains(ip) {
			c.Debugf("skipping NTP peer id='%d', srcadr='%s'", id, addr)
			continue
		}

		seen[addr] = true

		if !c.peerAddr[addr] {
			c.peerAddr[addr] = true
			c.Debugf("new NTP peer id='%d', srcadr='%s': creating charts", id, addr)
			c.addPeerCharts(addr)
		}

		c.peerIDs = append(c.peerIDs, id)
	}

	for addr := range c.peerAddr {
		if !seen[addr] {
			delete(c.peerAddr, addr)
			c.Debugf("stale NTP peer srcadr='%s': removing charts", addr)
			c.removePeerCharts(addr)
		}
	}

	return nil
}
