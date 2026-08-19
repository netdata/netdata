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
	if payload.entriesFound && payload.activeCount != int64(len(payload.tunnels)) {
		errs = append(errs, fmt.Errorf("IPsec active tunnel count mismatch: ntun=%d entries=%d; per-tunnel lifetime metrics may be incomplete", payload.activeCount, len(payload.tunnels)))
	}
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
	var activeCount int64
	if strings.TrimSpace(result.NTun) != "" {
		count, err := parseRequiredPANOSIntField("IPsec active tunnel count", result.NTun)
		if err != nil {
			return ipsecTunnelPayload{found: true, entriesFound: result.Entries != nil}, err
		}
		activeCount = count
	}
	if result.Entries == nil {
		payload.activeCount = activeCount
		return payload, nil
	}
	if strings.TrimSpace(result.NTun) == "" {
		activeCount = int64(len(result.Entries.Entries))
	}
	payload.tunnels = result.Entries.Entries
	payload.activeCount = activeCount
	return payload, nil
}

func ipsecTunnelKey(tunnel ipsecTunnel) string {
	return cleanID(firstNonEmpty(tunnel.Name, "unknown") + "_" + tunnel.Gateway + "_" + tunnel.Remote + "_" + firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI))
}
