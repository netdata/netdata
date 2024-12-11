// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDeviceNetworkIO = module.Priority + iota
	prioDevicePeers
	prioPeerNetworkIO
	prioPeerLatestHandShake
)

var (
	deviceChartsTmpl = module.Charts{
		deviceNetworkIOChartTmpl.Copy(),
		devicePeersChartTmpl.Copy(),
	}

	deviceNetworkIOChartTmpl = module.Chart{
		ID:       "device_%s_network_io",
		Title:    "Device traffic",
		Units:    "B/s",
		Fam:      "device traffic",
		Ctx:      "wireguard.device_network_io",
		Type:     module.Area,
		Priority: prioDeviceNetworkIO,
		Dims: module.Dims{
			{ID: "device_%s_receive", Name: "receive", Algo: module.Incremental},
			{ID: "device_%s_transmit", Name: "transmit", Algo: module.Incremental, Mul: -1},
		},
	}
	devicePeersChartTmpl = module.Chart{
		ID:       "device_%s_peers",
		Title:    "Device peers",
		Units:    "peers",
		Fam:      "device peers",
		Ctx:      "wireguard.device_peers",
		Priority: prioDevicePeers,
		Dims: module.Dims{
			{ID: "device_%s_peers", Name: "peers"},
		},
	}
)

var (
	peerChartsTmpl = module.Charts{
		peerNetworkIOChartTmpl.Copy(),
		peerLatestHandShakeChartTmpl.Copy(),
	}

	peerNetworkIOChartTmpl = module.Chart{
		ID:       "peer_%s_network_io",
		Title:    "Peer traffic",
		Units:    "B/s",
		Fam:      "peer traffic",
		Ctx:      "wireguard.peer_network_io",
		Type:     module.Area,
		Priority: prioPeerNetworkIO,
		Dims: module.Dims{
			{ID: "peer_%s_receive", Name: "receive", Algo: module.Incremental},
			{ID: "peer_%s_transmit", Name: "transmit", Algo: module.Incremental, Mul: -1},
		},
	}
	peerLatestHandShakeChartTmpl = module.Chart{
		ID:       "peer_%s_latest_handshake_ago",
		Title:    "Peer time elapsed since the latest handshake",
		Units:    "seconds",
		Fam:      "peer latest handshake",
		Ctx:      "wireguard.peer_latest_handshake_ago",
		Priority: prioPeerLatestHandShake,
		Dims: module.Dims{
			{ID: "peer_%s_latest_handshake_ago", Name: "time"},
		},
	}
)

func newDeviceCharts(device string) *module.Charts {
	charts := deviceChartsTmpl.Copy()

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, device)
		c.Labels = []module.Label{
			{Key: "device", Value: device},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, device)
		}
	}

	return charts
}

func (c *Collector) addNewDeviceCharts(device string) {
	charts := newDeviceCharts(device)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDeviceCharts(device string) {
	prefix := fmt.Sprintf("device_%s", device)

	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

func newPeerCharts(id, device, pubKey string) *module.Charts {
	charts := peerChartsTmpl.Copy()

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, id)
		c.Labels = []module.Label{
			{Key: "device", Value: device},
			{Key: "public_key", Value: pubKey},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, id)
		}
	}

	return charts
}

func (c *Collector) addNewPeerCharts(id, device, pubKey string) {
	charts := newPeerCharts(id, device, pubKey)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removePeerCharts(id string) {
	prefix := fmt.Sprintf("peer_%s", id)

	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}
