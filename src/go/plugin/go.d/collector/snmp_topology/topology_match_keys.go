// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"
)

func canonicalMatchKey(match topologyMatch) string {
	if key := canonicalPrimaryMACListKey(match); key != "" {
		return "mac:" + key
	}
	if key := canonicalHardwareListKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func topologyLinkSortKey(link topologyLink) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalMatchKey(link.Src.Match),
		canonicalMatchKey(link.Dst.Match),
		topologyEndpointKey(link.Src, "if_index"),
		topologyEndpointKey(link.Src, "if_name"),
		topologyEndpointKey(link.Src, "port_id"),
		topologyEndpointKey(link.Dst, "if_index"),
		topologyEndpointKey(link.Dst, "if_name"),
		topologyEndpointKey(link.Dst, "port_id"),
		link.State,
	}, "|")
}

func topologyEndpointKey(endpoint topologyLinkEndpoint, key string) string {
	switch key {
	case "if_index":
		if endpoint.IfIndex <= 0 {
			return ""
		}
		return strconv.Itoa(endpoint.IfIndex)
	case "if_name":
		return strings.TrimSpace(endpoint.IfName)
	case "port_id":
		return strings.TrimSpace(endpoint.PortID)
	case "port_name":
		return strings.TrimSpace(endpoint.PortName)
	default:
		return ""
	}
}
