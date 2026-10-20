// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type vpnFlowResult struct {
	IPSec *vpnFlowEntries `xml:"IPSec"`
}

type vpnFlowEntries struct {
	Entries []vpnFlowTunnel `xml:"entry"`
}

type vpnFlowTunnel struct {
	Name  string `xml:"name"`
	ID    string `xml:"id"`
	State string `xml:"state"`
}

type vpnGatewayResult struct {
	Entries *vpnGatewayEntries `xml:"entries"`
}

type vpnGatewayEntries struct {
	Entries []vpnGateway `xml:"entry"`
}

type vpnGateway struct {
	ID   string              `xml:"id"`
	Name string              `xml:"name"`
	V1   *vpnGatewayProtocol `xml:"v1"`
	V2   *vpnGatewayProtocol `xml:"v2"`
}

// vpnGatewayProtocol marks which IKE version a gateway is configured for; only its presence is used.
type vpnGatewayProtocol struct{}

type vpnIKESAResult struct {
	Entries []vpnIKESA `xml:"entry"`
}

type vpnIKESA struct {
	GatewayID string `xml:"gwid"`
	Name      string `xml:"name"`
}

// collectIPSecTunnelStateMetrics reports the state of every configured IPsec tunnel.
// Unlike the SA list, show vpn flow includes tunnels that have no established SA.
func (c *Collector) collectIPSecTunnelStateMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecFlowCommand)
	if err != nil {
		return false, fmt.Errorf("ipsec tunnel state metricset: %s API call: %w", panosCommandName(ipsecFlowCommand), err)
	}

	tunnels, found, err := parseVPNFlow(body)
	if err != nil {
		return false, fmt.Errorf("ipsec tunnel state metricset: %s response: %w", panosCommandName(ipsecFlowCommand), err)
	}
	if !found {
		return false, fmt.Errorf("ipsec tunnel state metricset: %s response: %w", panosCommandName(ipsecFlowCommand), missingPANOSResultError{expected: "<IPSec>"})
	}

	var errs []error
	var hasMetrics bool
	for _, tunnel := range tunnels {
		name := strings.TrimSpace(tunnel.Name)
		if name == "" {
			errs = append(errs, errors.New("IPsec flow entry: missing tunnel name"))
			continue
		}
		state := normalizeIPSecTunnelState(tunnel.State)
		if state == "" {
			errs = append(errs, fmt.Errorf("IPsec tunnel %s state: missing status", name))
			continue
		}
		observeStateSetVec(c.metrics.ipsec.tunnelState, state, ipsecTunnelStateLabelValues(tunnel)...)
		hasMetrics = true
	}
	return hasMetrics, errors.Join(errs...)
}

// collectIPSecGatewayMetrics reports IKE (phase 1) state per configured gateway.
// show vpn ike-sa lists only gateways that have an SA, so the gateway list is the
// source of truth and a configured gateway missing from the SA list is down.
func (c *Collector) collectIPSecGatewayMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecGatewayCommand)
	if err != nil {
		return false, fmt.Errorf("ipsec gateway metricset: %s API call: %w", panosCommandName(ipsecGatewayCommand), err)
	}

	gateways, found, err := parseVPNGateways(body)
	if err != nil {
		return false, fmt.Errorf("ipsec gateway metricset: %s response: %w", panosCommandName(ipsecGatewayCommand), err)
	}
	if !found {
		return false, fmt.Errorf("ipsec gateway metricset: %s response: %w", panosCommandName(ipsecGatewayCommand), missingPANOSResultError{expected: "<entries>"})
	}
	if len(gateways) == 0 {
		return false, nil
	}

	// A failed SA query leaves the gateway state as a gap. Reporting every gateway down here
	// would turn an API error into a false outage.
	body, err = c.apiClient.op(ctx, ipsecIKESACommand)
	if err != nil {
		return false, fmt.Errorf("ipsec gateway metricset: %s API call: %w", panosCommandName(ipsecIKESACommand), err)
	}
	sas, err := parseVPNIKESAs(body)
	if err != nil {
		return false, fmt.Errorf("ipsec gateway metricset: %s response: %w", panosCommandName(ipsecIKESACommand), err)
	}

	var errs []error
	var hasMetrics bool
	for _, gateway := range gateways {
		if strings.TrimSpace(gateway.Name) == "" {
			errs = append(errs, errors.New("IPsec gateway entry: missing gateway name"))
			continue
		}
		observeStateSetVec(
			c.metrics.ipsec.gatewayState,
			boolState(gatewayHasIKESA(gateway, sas), "up", "down"),
			ipsecGatewayLabelValues(gateway)...,
		)
		hasMetrics = true
	}
	return hasMetrics, errors.Join(errs...)
}

func gatewayHasIKESA(gateway vpnGateway, sas []vpnIKESA) bool {
	id, name := strings.TrimSpace(gateway.ID), strings.TrimSpace(gateway.Name)
	for _, sa := range sas {
		if id != "" && id == strings.TrimSpace(sa.GatewayID) {
			return true
		}
		if name == strings.TrimSpace(sa.Name) {
			return true
		}
	}
	return false
}

// parseVPNFlow returns the tunnels in a show vpn flow response. found is false when the
// response has no <IPSec> section, so an unrecognized payload is not read as zero tunnels.
func parseVPNFlow(body []byte) ([]vpnFlowTunnel, bool, error) {
	var result vpnFlowResult
	if err := decodePANOSResult(body, "PAN-OS IPsec flow response", &result); err != nil {
		return nil, false, err
	}
	if result.IPSec == nil {
		return nil, false, nil
	}
	return result.IPSec.Entries, true, nil
}

func parseVPNGateways(body []byte) ([]vpnGateway, bool, error) {
	var result vpnGatewayResult
	if err := decodePANOSResult(body, "PAN-OS IPsec gateway response", &result); err != nil {
		return nil, false, err
	}
	if result.Entries == nil {
		return nil, false, nil
	}
	return result.Entries.Entries, true, nil
}

// parseVPNIKESAs returns the IKE SAs in a show vpn ike-sa response. An empty result is a
// valid response meaning no gateway has an SA, so there is no "not found" case.
func parseVPNIKESAs(body []byte) ([]vpnIKESA, error) {
	var result vpnIKESAResult
	if err := decodePANOSResult(body, "PAN-OS IPsec IKE SA response", &result); err != nil {
		return nil, err
	}
	return result.Entries, nil
}

func normalizeIPSecTunnelState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "":
		return ""
	case "active":
		return "active"
	case "inactive":
		return "inactive"
	default:
		return "unknown"
	}
}

func ipsecIKEVersion(gateway vpnGateway) string {
	switch {
	case gateway.V1 != nil && gateway.V2 == nil:
		return "ikev1"
	case gateway.V2 != nil && gateway.V1 == nil:
		return "ikev2"
	default:
		return "unknown"
	}
}
