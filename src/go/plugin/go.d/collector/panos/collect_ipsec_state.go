// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type vpnFlowResult struct {
	IPSec *vpnFlowEntries `xml:"IPSec"`
}

type vpnFlowEntries struct {
	Entries []vpnFlowTunnel        `xml:"entry"`
	Text    string                 `xml:",chardata"`
	Other   []vpnUnexpectedElement `xml:",any"`
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
	Entries []vpnGateway           `xml:"entry"`
	Text    string                 `xml:",chardata"`
	Other   []vpnUnexpectedElement `xml:",any"`
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
	Created   string `xml:"created"`
	Expires   string `xml:"expires"`
}

// collectIPSecTunnelStateMetrics reports the state of every configured IPsec tunnel.
// Unlike the SA list, show vpn flow includes tunnels that have no established SA.
func (c *Collector) collectIPSecTunnelStateMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecFlowCommand)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.tunnelState, ipsecTunnelStates, c.ipsecTunnelInventory)
		return false, fmt.Errorf("ipsec tunnel state metricset: %s API call: %w", panosCommandName(ipsecFlowCommand), err)
	}

	tunnels, err := parseVPNFlow(body)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.tunnelState, ipsecTunnelStates, c.ipsecTunnelInventory)
		return false, fmt.Errorf("ipsec tunnel state metricset: %s response: %w", panosCommandName(ipsecFlowCommand), err)
	}

	// Membership changes only after the complete inventory has been validated.
	inventory := make(map[[2]string]metrix.LabelSet, len(tunnels))
	for _, tunnel := range tunnels {
		key := ipsecTunnelStateKey(tunnel)
		labels, ok := c.ipsecTunnelInventory[key]
		if !ok {
			labels = c.store.Write().SnapshotMeter("").LabelSet(
				metrix.Label{Key: "tunnel", Value: key[0]},
				metrix.Label{Key: "tunnel_id", Value: key[1]},
			)
		}
		inventory[key] = labels
	}
	c.ipsecTunnelInventory = inventory

	var errs []error
	var hasMetrics bool
	for _, tunnel := range tunnels {
		state := normalizeIPSecTunnelState(tunnel.State)
		if state == "" {
			errs = append(errs, fmt.Errorf("IPsec tunnel %s state: missing status", tunnel.Name))
		} else {
			hasMetrics = true
		}
		observeIPSecState(c.metrics.ipsec.tunnelState, ipsecTunnelStates, state, inventory[ipsecTunnelStateKey(tunnel)])
	}
	return hasMetrics, errors.Join(errs...)
}

// collectIPSecGatewayMetrics reports IKE (phase 1) state per configured gateway.
// A configured gateway missing from a validated show vpn ike-sa list is down.
// This is independent of the child SA and tunnel flow state.
func (c *Collector) collectIPSecGatewayMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, ipsecGatewayCommand)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.gatewayState, ipsecGatewayStates, c.ipsecGatewayInventory)
		return false, fmt.Errorf("ipsec gateway metricset: %s API call: %w", panosCommandName(ipsecGatewayCommand), err)
	}

	gateways, err := parseVPNGateways(body)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.gatewayState, ipsecGatewayStates, c.ipsecGatewayInventory)
		return false, fmt.Errorf("ipsec gateway metricset: %s response: %w", panosCommandName(ipsecGatewayCommand), err)
	}
	inventory := make(map[[2]string]metrix.LabelSet, len(gateways))
	for _, gateway := range gateways {
		key := ipsecGatewayStateKey(gateway)
		labels, ok := c.ipsecGatewayInventory[key]
		if !ok {
			labels = c.store.Write().SnapshotMeter("").LabelSet(
				metrix.Label{Key: "gateway", Value: key[0]},
				metrix.Label{Key: "ike_version", Value: key[1]},
			)
		}
		inventory[key] = labels
	}
	c.ipsecGatewayInventory = inventory
	if len(inventory) == 0 {
		return false, nil
	}

	// A failed SA query leaves the gateway state as a gap. Reporting every gateway down here
	// would turn an API error into a false outage.
	body, err = c.apiClient.op(ctx, ipsecIKESACommand)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.gatewayState, ipsecGatewayStates, inventory)
		return false, fmt.Errorf("ipsec gateway metricset: %s API call: %w", panosCommandName(ipsecIKESACommand), err)
	}
	sas, err := parseVPNIKESAs(body)
	if err != nil {
		observeIPSecUnavailable(c.metrics.ipsec.gatewayState, ipsecGatewayStates, inventory)
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

	for _, gateway := range gateways {
		observeIPSecState(
			c.metrics.ipsec.gatewayState,
			ipsecGatewayStates,
			boolState(byID[gateway.ID] || byName[gateway.Name], "up", "down"),
			inventory[ipsecGatewayStateKey(gateway)],
		)
	}
	return true, nil
}

func parseVPNFlow(body []byte) ([]vpnFlowTunnel, error) {
	var result vpnFlowResult
	if err := decodePANOSResult(body, "PAN-OS IPsec flow response", &result); err != nil {
		return nil, err
	}
	if result.IPSec == nil {
		return nil, missingPANOSResultError{expected: "<IPSec>"}
	}
	if err := validateVPNEntryList(result.IPSec.Text, result.IPSec.Other); err != nil {
		return nil, fmt.Errorf("parse PAN-OS IPsec flow response result: %w", err)
	}
	for i := range result.IPSec.Entries {
		tunnel := &result.IPSec.Entries[i]
		tunnel.Name = strings.TrimSpace(tunnel.Name)
		if tunnel.Name == "" {
			return nil, errors.New("IPsec flow entry: missing tunnel name")
		}
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
	if err := validateVPNEntryList(result.Entries.Text, result.Entries.Other); err != nil {
		return nil, fmt.Errorf("parse PAN-OS IPsec gateway response result: %w", err)
	}
	for i := range result.Entries.Entries {
		gateway := &result.Entries.Entries[i]
		gateway.ID = strings.TrimSpace(gateway.ID)
		gateway.Name = strings.TrimSpace(gateway.Name)
		if gateway.Name == "" {
			return nil, errors.New("IPsec gateway entry: missing gateway name")
		}
	}
	return result.Entries.Entries, nil
}

// parseVPNIKESAs returns the IKE SAs in a show vpn ike-sa response. An empty result is a
// valid response meaning no gateway has an SA, so there is no "not found" case. Unsupported
// or incomplete results are rejected before their absence can imply that a gateway is down.
func parseVPNIKESAs(body []byte) ([]vpnIKESA, error) {
	var result vpnIKESAResult
	if err := decodePANOSResult(body, "PAN-OS IPsec IKE SA response", &result); err != nil {
		return nil, err
	}
	if err := validateVPNEntryList(result.Text, result.Other); err != nil {
		return nil, fmt.Errorf("parse PAN-OS IPsec IKE SA response result: %w", err)
	}
	for i := range result.Entries {
		sa := &result.Entries[i]
		sa.GatewayID = strings.TrimSpace(sa.GatewayID)
		sa.Name = strings.TrimSpace(sa.Name)
		if sa.GatewayID == "" && sa.Name == "" {
			// An unidentifiable SA can belong to any gateway, so absence cannot establish down.
			return nil, errors.New("parse PAN-OS IPsec IKE SA response result: entry has no gateway identity")
		}
		sa.Created = strings.TrimSpace(sa.Created)
		sa.Expires = strings.TrimSpace(sa.Expires)
		if sa.Created == "" || sa.Expires == "" {
			// Established rows in the supported captures have both times. An incomplete row
			// does not establish either health or absence; leave the metricset as a gap.
			return nil, errors.New("parse PAN-OS IPsec IKE SA response result: entry has missing establishment or expiration time")
		}
	}
	return result.Entries, nil
}

func validateVPNEntryList(text string, other []vpnUnexpectedElement) error {
	if len(other) > 0 {
		return fmt.Errorf("unexpected element <%s>", other[0].XMLName.Local)
	}
	if strings.TrimSpace(text) != "" {
		return errors.New("unexpected text")
	}
	return nil
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
