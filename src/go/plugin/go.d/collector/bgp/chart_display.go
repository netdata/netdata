// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"strings"
)

func familyDisplay(f familyStats) string {
	return fmt.Sprintf("%s / %s / %s", chartScopeDisplay(f.Backend, f.VRF, f.Table), chartAFIDisplay(f.AFI), chartSAFIDisplay(f.SAFI))
}

func peerDisplay(p peerStats) string {
	return fmt.Sprintf("%s peer %s", familyDisplay(p.Family), p.Address)
}

func neighborDisplay(n neighborStats) string {
	return fmt.Sprintf("%s neighbor %s", chartScopeDisplay(n.Backend, n.VRF, n.Table), n.Address)
}

func rpkiCacheDisplay(cache rpkiCacheStats) string {
	return fmt.Sprintf("%s RPKI cache %s", backendDisplay(cache.Backend), cache.Name)
}

func rpkiInventoryDisplay(inv rpkiInventoryStats) string {
	return fmt.Sprintf("%s RPKI inventory %s", backendDisplay(inv.Backend), inv.Scope)
}

func vniDisplay(vni vniStats) string {
	return fmt.Sprintf("vrf %s EVPN VNI %d", emptyToDefault(vni.TenantVRF), vni.VNI)
}

func chartScopeDisplay(backend, vrf, table string) string {
	return fmt.Sprintf("%s %s", chartScopeKind(backend), chartScopeName(backend, vrf, table))
}

func chartScopeKind(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case backendBIRD:
		return "table"
	default:
		return "vrf"
	}
}

func chartScopeName(backend, vrf, table string) string {
	if chartScopeKind(backend) == "table" {
		if strings.TrimSpace(table) != "" {
			return strings.TrimSpace(table)
		}
		if strings.TrimSpace(vrf) != "" {
			return strings.TrimSpace(vrf)
		}
		return "default"
	}

	if strings.TrimSpace(vrf) != "" {
		return strings.TrimSpace(vrf)
	}
	if strings.TrimSpace(table) != "" {
		return strings.TrimSpace(table)
	}
	return "default"
}

func backendDisplay(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case backendFRR:
		return "FRR"
	case backendBIRD:
		return "BIRD"
	case backendGoBGP:
		return "GoBGP"
	case backendOpenBGPD:
		return "OpenBGPD"
	default:
		return strings.TrimSpace(backend)
	}
}

func chartAFIDisplay(afi string) string {
	switch strings.ToLower(strings.TrimSpace(afi)) {
	case "ipv4":
		return "IPv4"
	case "ipv6":
		return "IPv6"
	case "l2vpn":
		return "L2VPN"
	default:
		return strings.ToUpper(strings.TrimSpace(afi))
	}
}

func chartSAFIDisplay(safi string) string {
	switch strings.ToLower(strings.TrimSpace(safi)) {
	case "unicast":
		return "unicast"
	case "multicast":
		return "multicast"
	case "vpn":
		return "VPN"
	case "evpn":
		return "EVPN"
	case "flowspec":
		return "FlowSpec"
	case "mpls":
		return "MPLS"
	default:
		return strings.TrimSpace(safi)
	}
}

func familyLabelValue(afi, safi string) string {
	afi = strings.ToLower(strings.TrimSpace(afi))
	safi = strings.ToLower(strings.TrimSpace(safi))

	switch {
	case afi == "" && safi == "":
		return ""
	case safi == "":
		return afi
	case afi == "":
		return safi
	default:
		return afi + "/" + safi
	}
}

func emptyToDefault(v string) string {
	if strings.TrimSpace(v) == "" {
		return "default"
	}
	return strings.TrimSpace(v)
}
