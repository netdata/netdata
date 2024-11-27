// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"fmt"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.client == nil {
		client, err := c.newWGClient()
		if err != nil {
			return nil, fmt.Errorf("creating WireGuard client: %v", err)
		}
		c.client = client
	}

	// TODO: probably we need to get a list of interfaces and query interfaces using client.Device()
	// https://github.com/WireGuard/wgctrl-go/blob/3d4a969bb56bb6931f6661af606bc9c4195b4249/internal/wglinux/client_linux.go#L79-L80
	devices, err := c.client.Devices()
	if err != nil {
		return nil, fmt.Errorf("retrieving WireGuard devices: %v", err)
	}

	if len(devices) == 0 {
		c.Info("no WireGuard devices found on the host system")
	}

	now := time.Now()
	if c.cleanupLastTime.IsZero() {
		c.cleanupLastTime = now
	}

	mx := make(map[string]int64)

	c.collectDevicesPeers(mx, devices, now)

	if now.Sub(c.cleanupLastTime) > c.cleanupEvery {
		c.cleanupLastTime = now
		c.cleanupDevicesPeers(devices)
	}

	return mx, nil
}

func (c *Collector) collectDevicesPeers(mx map[string]int64, devices []*wgtypes.Device, now time.Time) {
	for _, d := range devices {
		if !c.devices[d.Name] {
			c.devices[d.Name] = true
			c.addNewDeviceCharts(d.Name)
		}

		mx["device_"+d.Name+"_peers"] = int64(len(d.Peers))
		if len(d.Peers) == 0 {
			mx["device_"+d.Name+"_receive"] = 0
			mx["device_"+d.Name+"_transmit"] = 0
			continue
		}

		for _, p := range d.Peers {
			if p.LastHandshakeTime.IsZero() {
				continue
			}

			pubKey := p.PublicKey.String()
			id := peerID(d.Name, pubKey)

			if !c.peers[id] {
				c.peers[id] = true
				c.addNewPeerCharts(id, d.Name, pubKey)
			}

			mx["device_"+d.Name+"_receive"] += p.ReceiveBytes
			mx["device_"+d.Name+"_transmit"] += p.TransmitBytes
			mx["peer_"+id+"_receive"] = p.ReceiveBytes
			mx["peer_"+id+"_transmit"] = p.TransmitBytes
			mx["peer_"+id+"_latest_handshake_ago"] = int64(now.Sub(p.LastHandshakeTime).Seconds())
		}
	}
}

func (c *Collector) cleanupDevicesPeers(devices []*wgtypes.Device) {
	seenDevices, seenPeers := make(map[string]bool), make(map[string]bool)
	for _, d := range devices {
		seenDevices[d.Name] = true
		for _, p := range d.Peers {
			seenPeers[peerID(d.Name, p.PublicKey.String())] = true
		}
	}
	for d := range c.devices {
		if !seenDevices[d] {
			delete(c.devices, d)
			c.removeDeviceCharts(d)
		}
	}
	for p := range c.peers {
		if !seenPeers[p] {
			delete(c.peers, p)
			c.removePeerCharts(p)
		}
	}
}

func peerID(device, peerPublicKey string) string {
	return device + "_" + peerPublicKey
}
