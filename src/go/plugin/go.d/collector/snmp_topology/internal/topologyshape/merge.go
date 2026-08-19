// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func mergeTopologyMatch(base, other topologymodel.Match) topologymodel.Match {
	base.ChassisIDs = appendUniqueTopologyStrings(base.ChassisIDs, other.ChassisIDs...)
	base.MacAddresses = appendUniqueTopologyStrings(base.MacAddresses, other.MacAddresses...)
	base.IPAddresses = appendUniqueTopologyStrings(base.IPAddresses, other.IPAddresses...)
	base.Hostnames = appendUniqueTopologyStrings(base.Hostnames, other.Hostnames...)
	base.DNSNames = appendUniqueTopologyStrings(base.DNSNames, other.DNSNames...)
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base
}

type topologyMatchCollapseLists struct {
	chassisIDs   []string
	macAddresses []string
	ipAddresses  []string
	hostnames    []string
	dnsNames     []string
}

func (a *topologyMatchCollapseLists) add(match topologymodel.Match) {
	a.chassisIDs = append(a.chassisIDs, match.ChassisIDs...)
	a.macAddresses = append(a.macAddresses, match.MacAddresses...)
	a.ipAddresses = append(a.ipAddresses, match.IPAddresses...)
	a.hostnames = append(a.hostnames, match.Hostnames...)
	a.dnsNames = append(a.dnsNames, match.DNSNames...)
}

func (a *topologyMatchCollapseLists) apply(match *topologymodel.Match) {
	match.ChassisIDs = appendUniqueTopologyStrings(nil, a.chassisIDs...)
	match.MacAddresses = appendUniqueTopologyStrings(nil, a.macAddresses...)
	match.IPAddresses = appendUniqueTopologyStrings(nil, a.ipAddresses...)
	match.Hostnames = appendUniqueTopologyStrings(nil, a.hostnames...)
	match.DNSNames = appendUniqueTopologyStrings(nil, a.dnsNames...)
}

func clearTopologyMatchCollapseLists(match *topologymodel.Match) {
	match.ChassisIDs = nil
	match.MacAddresses = nil
	match.IPAddresses = nil
	match.Hostnames = nil
	match.DNSNames = nil
}

func mergeTopologyStringMap(base, other map[string]string) map[string]string {
	if len(other) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(other))
	}
	for key, value := range other {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func appendUniqueTopologyStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
