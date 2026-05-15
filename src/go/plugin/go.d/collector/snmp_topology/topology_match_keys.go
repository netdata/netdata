// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func canonicalMatchKey(match topology.Match) string {
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

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalMatchKey(link.Src.Match),
		canonicalMatchKey(link.Dst.Match),
		attrKey(link.Src.Attributes, "if_index"),
		attrKey(link.Src.Attributes, "if_name"),
		attrKey(link.Src.Attributes, "port_id"),
		attrKey(link.Dst.Attributes, "if_index"),
		attrKey(link.Dst.Attributes, "if_name"),
		attrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, "|")
}

func attrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
