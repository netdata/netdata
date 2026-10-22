// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ipsecResult struct {
	NTun    string        `xml:"ntun"`
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
	ISPI       string `xml:"i_spi"`
	OSPI       string `xml:"o_spi"`
}

func (c *Collector) collectIPSecMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecSACommand)
	if err != nil {
		return false, fmt.Errorf("ipsec metricset: %s API call: %w", panosCommandName(ipsecSACommand), err)
	}

	payload, err := parseIPSecTunnels(body)
	if err != nil {
		return false, fmt.Errorf("ipsec metricset: %s response: %w", panosCommandName(ipsecSACommand), err)
	}
	if !payload.found {
		return false, fmt.Errorf("ipsec metricset: %s response: %w", panosCommandName(ipsecSACommand), missingPANOSResultError{expected: "<ntun> or <entries>"})
	}

	c.metrics.ipsec.tunnelsActive.Observe(float64(payload.activeCount))

	var errs []error
	for _, tunnel := range payload.tunnels {
		key := ipsecTunnelKey(tunnel)
		value, err := parseRequiredPANOSIntField("IPsec tunnel "+firstNonEmpty(tunnel.Name, key)+" remain", tunnel.Remain)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.metrics.ipsec.saLifetime.WithLabelValues(ipsecTunnelLabelValues(tunnel)...).Observe(float64(value))
	}
	return true, errors.Join(errs...)
}

type ipsecTunnelPayload struct {
	tunnels      []ipsecTunnel
	activeCount  int64
	found        bool
	entriesFound bool
}

func parseIPSecTunnels(body []byte) (ipsecTunnelPayload, error) {
	var result ipsecResult
	if err := decodePANOSResult(body, "PAN-OS IPsec response", &result); err != nil {
		return ipsecTunnelPayload{}, err
	}
	if result.Entries == nil && strings.TrimSpace(result.NTun) == "" {
		return ipsecTunnelPayload{}, nil
	}

	payload := ipsecTunnelPayload{
		found:        true,
		entriesFound: result.Entries != nil,
	}
	if result.Entries != nil {
		// <ntun> is the number of configured tunnels, not established SAs (PAN-OS lists
		// only established SAs in <entries>), so it is ignored when entries are present.
		payload.tunnels = result.Entries.Entries
		payload.activeCount = countIPSecTunnels(payload.tunnels)
		return payload, nil
	}

	count, err := parseRequiredPANOSIntField("IPsec active tunnel count", result.NTun)
	if err != nil {
		return ipsecTunnelPayload{found: true}, err
	}
	payload.activeCount = count
	return payload, nil
}

// ipsecTunnelIdentity identifies a tunnel across rekeys. SPIs are left out on purpose: an old SA
// and its replacement have different SPIs but belong to the same tunnel.
type ipsecTunnelIdentity struct {
	name, gateway, tunnelID string
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

func ipsecTunnelKey(tunnel ipsecTunnel) string {
	return cleanID(firstNonEmpty(tunnel.Name, "unknown") + "_" + tunnel.Gateway + "_" + tunnel.Remote + "_" + firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI))
}
