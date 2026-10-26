// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"encoding/xml"
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
	Text    string     `xml:",chardata"`
	// Other collects any child that is not an <entry>, such as an <error> element.
	Other []vpnUnexpectedElement `xml:",any"`
}

type vpnUnexpectedElement struct {
	XMLName xml.Name
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

	tunnels, err := parseVPNFlow(body)
	if err != nil {
		return false, fmt.Errorf("ipsec tunnel state metricset: %s response: %w", panosCommandName(ipsecFlowCommand), err)
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

	gateways, err := parseVPNGateways(body)
	if err != nil {
		return false, fmt.Errorf("ipsec gateway metricset: %s response: %w", panosCommandName(ipsecGatewayCommand), err)
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
	byID := make(map[string]bool, len(sas))
	byName := make(map[string]bool, len(sas))
	for _, sa := range sas {
		if sa.GatewayID != "" {
			byID[sa.GatewayID] = true
		}
		if sa.Name != "" {
			byName[sa.Name] = true
		}
	}

	var errs []error
	var hasMetrics bool
	for _, gateway := range gateways {
		if gateway.Name == "" {
			errs = append(errs, errors.New("IPsec gateway entry: missing gateway name"))
			continue
		}
		observeStateSetVec(
			c.metrics.ipsec.gatewayState,
			boolState(byID[gateway.ID] || byName[gateway.Name], "up", "down"),
			ipsecGatewayLabelValues(gateway)...,
		)
		hasMetrics = true
	}
	return hasMetrics, errors.Join(errs...)
}

func parseVPNFlow(body []byte) ([]vpnFlowTunnel, error) {
	var result vpnFlowResult
	if err := decodePANOSResult(body, "PAN-OS IPsec flow response", &result); err != nil {
		return nil, err
	}
	if result.IPSec == nil {
		return nil, missingPANOSResultError{expected: "<IPSec>"}
	}
	return result.IPSec.Entries, nil
}

func parseVPNGateways(body []byte) ([]vpnGateway, error) {
	var result vpnGatewayResult
	if err := decodePANOSResult(body, "PAN-OS IPsec gateway response", &result); err != nil {
		return nil, err
	}
	if result.Entries == nil {
		return nil, missingPANOSResultError{expected: "<entries>"}
	}
	for i := range result.Entries.Entries {
		gateway := &result.Entries.Entries[i]
		gateway.ID = strings.TrimSpace(gateway.ID)
		gateway.Name = strings.TrimSpace(gateway.Name)
	}
	return result.Entries.Entries, nil
}

// parseVPNIKESAs returns the IKE SAs in a show vpn ike-sa response. An empty result is a
// valid response meaning no gateway has an SA, so there is no "not found" case. Any child
// other than <entry> is rejected: reading it as an empty list would mark every gateway down.
func parseVPNIKESAs(body []byte) ([]vpnIKESA, error) {
	var result vpnIKESAResult
	if err := decodePANOSResult(body, "PAN-OS IPsec IKE SA response", &result); err != nil {
		return nil, err
	}
	if len(result.Other) > 0 {
		return nil, fmt.Errorf("parse PAN-OS IPsec IKE SA response result: unexpected element <%s>", result.Other[0].XMLName.Local)
	}
	if strings.TrimSpace(result.Text) != "" {
		return nil, errors.New("parse PAN-OS IPsec IKE SA response result: unexpected text")
	}
	for i := range result.Entries {
		sa := &result.Entries[i]
		sa.GatewayID = strings.TrimSpace(sa.GatewayID)
		sa.Name = strings.TrimSpace(sa.Name)
		if sa.GatewayID == "" && sa.Name == "" {
			// An unidentifiable SA can belong to any gateway, so absence cannot establish down.
			return nil, errors.New("parse PAN-OS IPsec IKE SA response result: entry has no gateway identity")
		}
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
	case "init":
		return "init"
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
