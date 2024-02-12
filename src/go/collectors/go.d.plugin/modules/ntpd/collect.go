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

func (n *NTPd) collect() (map[string]int64, error) {
	if n.client == nil {
		client, err := n.newClient(n.Config)
		if err != nil {
			return nil, fmt.Errorf("creating NTP client: %v", err)
		}
		n.client = client
	}

	mx := make(map[string]int64)

	if err := n.collectInfo(mx); err != nil {
		return nil, err
	}

	if n.CollectPeers {
		if now := time.Now(); now.Sub(n.findPeersTime) > n.findPeersEvery {
			n.findPeersTime = now
			if err := n.findPeers(); err != nil {
				n.Warning(err)
			}
		}
		n.collectPeersInfo(mx)
	}

	return mx, nil
}

func (n *NTPd) collectInfo(mx map[string]int64) error {
	info, err := n.client.systemInfo()
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

func (n *NTPd) collectPeersInfo(mx map[string]int64) {
	for _, id := range n.peerIDs {
		info, err := n.client.peerInfo(id)
		if err != nil {
			n.Warningf("error on querying NTP peer info id='%d': %v", id, err)
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

func (n *NTPd) findPeers() error {
	n.peerIDs = n.peerIDs[:0]

	n.Debug("querying NTP peers")
	peers, err := n.client.peerIDs()
	if err != nil {
		return fmt.Errorf("querying NTP peers: %v", err)
	}

	n.Debugf("found %d NTP peers (ids: %v)", len(peers), peers)
	seen := make(map[string]bool)

	for _, id := range peers {
		info, err := n.client.peerInfo(id)
		if err != nil {
			n.Debugf("error on querying NTP peer info id='%d': %v", id, err)
			continue
		}

		addr, ok := info["srcadr"]
		if ip := net.ParseIP(addr); !ok || ip == nil || n.peerIPAddrFilter.Contains(ip) {
			n.Debugf("skipping NTP peer id='%d', srcadr='%s'", id, addr)
			continue
		}

		seen[addr] = true

		if !n.peerAddr[addr] {
			n.peerAddr[addr] = true
			n.Debugf("new NTP peer id='%d', srcadr='%s': creating charts", id, addr)
			n.addPeerCharts(addr)
		}

		n.peerIDs = append(n.peerIDs, id)
	}

	for addr := range n.peerAddr {
		if !seen[addr] {
			delete(n.peerAddr, addr)
			n.Debugf("stale NTP peer srcadr='%s': removing charts", addr)
			n.removePeerCharts(addr)
		}
	}

	return nil
}
