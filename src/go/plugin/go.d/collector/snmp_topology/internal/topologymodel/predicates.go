// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func NormalizedMatchIPs(match Match) []string {
	if len(match.IPAddresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(match.IPAddresses))
	seen := make(map[string]struct{}, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		ip := topologyutil.NormalizeIPAddress(value)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func ActorIsInferred(actor Actor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
		return true
	}
	if actor.Detail.L2.Device.Inferred {
		return true
	}
	return false
}

func IsManagedSNMPDeviceActor(actor Actor) bool {
	if !topologyengine.IsDeviceActorType(actor.ActorType) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(actor.Source)) != "snmp" {
		return false
	}
	return !ActorIsInferred(actor)
}

func MatchLocalActor(match Match, local Device) bool {
	localChassisID := strings.TrimSpace(local.ChassisID)
	if localChassisID != "" {
		for _, chassisID := range match.ChassisIDs {
			if strings.EqualFold(strings.TrimSpace(chassisID), localChassisID) {
				return true
			}
		}
	}

	localSysName := strings.TrimSpace(local.SysName)
	if localSysName != "" && strings.EqualFold(strings.TrimSpace(match.SysName), localSysName) {
		return true
	}

	localIP := topologyutil.NormalizeIPAddress(local.ManagementIP)
	if localIP != "" {
		for _, ip := range match.IPAddresses {
			if topologyutil.NormalizeIPAddress(ip) == localIP {
				return true
			}
		}
	}

	return false
}
