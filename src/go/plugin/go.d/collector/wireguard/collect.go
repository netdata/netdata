// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"fmt"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func (c *Collector) collect() error {
	devices, err := c.devices()
	if err != nil {
		return err
	}

	c.writeMetrics(devices, c.now())

	return nil
}

func (c *Collector) devices() ([]*wgtypes.Device, error) {
	if c.client == nil {
		client, err := c.newWGClient()
		if err != nil {
			return nil, fmt.Errorf("creating WireGuard client: %v", err)
		}
		c.client = client
	}

	// Devices() fails as a whole when any single device cannot be read (for example an interface removed between
	// listing and querying), so one bad device costs the whole cycle. Querying each interface with Device() would
	// keep the others.
	devices, err := c.client.Devices()
	if err != nil {
		return nil, fmt.Errorf("retrieving WireGuard devices: %v", err)
	}

	return devices, nil
}

// writeMetrics builds the instruments every cycle: devices and peers come and go with the host's WireGuard
// configuration, so a long-lived Vec would keep a handle for every peer ever seen.
func (c *Collector) writeMetrics(devices []*wgtypes.Device, now time.Time) {
	meter := c.store.Write().SnapshotMeter("")
	deviceVec := meter.Vec("device")
	peerVec := meter.Vec("device", "public_key")

	devicePeers := deviceVec.Gauge("device_peers")
	deviceReceive := deviceVec.Counter("device_receive")
	deviceTransmit := deviceVec.Counter("device_transmit")
	peerReceive := peerVec.Counter("peer_receive")
	peerTransmit := peerVec.Counter("peer_transmit")
	peerLatestHandshakeAgo := peerVec.Gauge("peer_latest_handshake_ago")

	for _, d := range devices {
		devicePeers.WithLabelValues(d.Name).Observe(float64(len(d.Peers)))

		// Device traffic is the sum over peers that have completed a handshake. A device without peers reports
		// zero; a device whose peers have no handshake yet reports no traffic sample.
		var receive, transmit int64
		hasTraffic := len(d.Peers) == 0

		for _, p := range d.Peers {
			if p.LastHandshakeTime.IsZero() {
				continue
			}
			hasTraffic = true

			pubKey := p.PublicKey.String()
			receive += p.ReceiveBytes
			transmit += p.TransmitBytes

			peerReceive.WithLabelValues(d.Name, pubKey).ObserveTotal(float64(p.ReceiveBytes))
			peerTransmit.WithLabelValues(d.Name, pubKey).ObserveTotal(float64(p.TransmitBytes))
			peerLatestHandshakeAgo.WithLabelValues(d.Name, pubKey).Observe(float64(int64(now.Sub(p.LastHandshakeTime).Seconds())))
		}

		if hasTraffic {
			deviceReceive.WithLabelValues(d.Name).ObserveTotal(float64(receive))
			deviceTransmit.WithLabelValues(d.Name).ObserveTotal(float64(transmit))
		}
	}
}
