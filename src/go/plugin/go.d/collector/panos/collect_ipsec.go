// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ipsecResult struct {
	Entries *ipsecEntries `xml:"entries"`
}

type ipsecEntries struct {
	Entries []ipsecTunnel `xml:"entry"`
}

type ipsecTunnel struct {
	Name       string `xml:"name"`
	Gateway    string `xml:"gateway"`
	Remote     string `xml:"remote"`
	Protocol   string `xml:"proto"`
	Encryption string `xml:"enc"`
	Remain     string `xml:"remain"`
	TID        string `xml:"tid"`
}

func (c *Collector) collectIPSecMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecSACommand)
	if err != nil {
		return false, fmt.Errorf("ipsec metricset: %s API call: %w", panosCommandName(ipsecSACommand), err)
	}

	tunnels, err := parseIPSecTunnels(body)
	if err != nil {
		return false, fmt.Errorf("ipsec metricset: %s response: %w", panosCommandName(ipsecSACommand), err)
	}
	c.metrics.ipsec.tunnelsActive.Observe(float64(countIPSecTunnels(tunnels)))

	var errs []error
	lifetimes := make(map[[6]string]int64, len(tunnels))
	for _, tunnel := range tunnels {
		labels := ipsecTunnelLabelValues(tunnel)
		name := labelValue(firstNonEmpty(tunnel.Name, tunnel.TID, tunnel.Remote, tunnel.Gateway), "unknown")
		value, err := parseRequiredPANOSIntField("IPsec tunnel "+name+" remain", tunnel.Remain)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// Snapshot gauges overwrite repeated observations. Reduce overlapping SAs first.
		if previous, ok := lifetimes[labels]; !ok || value > previous {
			lifetimes[labels] = value
		}
	}
	for labels, value := range lifetimes {
		c.metrics.ipsec.saLifetime.WithLabelValues(labels[:]...).Observe(float64(value))
	}
	return true, errors.Join(errs...)
}

func parseIPSecTunnels(body []byte) ([]ipsecTunnel, error) {
	var result ipsecResult
	if err := decodePANOSResult(body, "PAN-OS IPsec response", &result); err != nil {
		return nil, err
	}
	// <ntun> counts configured tunnels; only an explicit <entries> list establishes
	// the active count, including zero for an empty list.
	if result.Entries == nil {
		return nil, missingPANOSResultError{
			expected: "<entries>",
		}
	}
	return result.Entries.Entries, nil
}

// ipsecTunnelIdentity identifies a tunnel across rekeys. SPIs are left out on purpose: an old SA
// and its replacement have different SPIs but belong to the same tunnel.
type ipsecTunnelIdentity struct {
	name     string
	gateway  string
	tunnelID string
}

// countIPSecTunnels counts distinct tunnels, so an SA that briefly coexists with
// its replacement during a rekey is not counted twice.
func countIPSecTunnels(tunnels []ipsecTunnel) int64 {
	seen := make(map[ipsecTunnelIdentity]struct{}, len(tunnels))
	for _, tunnel := range tunnels {
		seen[ipsecTunnelIdentity{
			name:     strings.TrimSpace(tunnel.Name),
			gateway:  strings.TrimSpace(tunnel.Gateway),
			tunnelID: strings.TrimSpace(tunnel.TID),
		}] = struct{}{}
	}
	return int64(len(seen))
}
