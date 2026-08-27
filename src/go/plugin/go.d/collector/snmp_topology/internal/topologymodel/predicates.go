// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func NormalizedMatchIPs(match Match) []string {
	out, _ := NormalizedMatchIPsWithLimiter(match, nil)
	return out
}

func NormalizedMatchIPsWithLimiter(match Match, limiter worklimit.Limiter) ([]string, error) {
	if len(match.IPAddresses) == 0 {
		return nil, nil
	}
	if err := worklimit.ChargeStrings(limiter, match.IPAddresses); err != nil {
		return nil, err
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
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return nil, err
	}
	return out, nil
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

func ActorSegmentKind(actor Actor) string {
	return strings.ToLower(strings.TrimSpace(actor.SegmentKind))
}

func ActorIsSegment(actor Actor) bool {
	return ActorSegmentKind(actor) != ""
}

func ActorIsL3SubnetSegment(actor Actor) bool {
	return ActorSegmentKind(actor) == SegmentKindL3Subnet
}
