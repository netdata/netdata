// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func mergeTopologyMatch(base, other topologymodel.Match) topologymodel.Match {
	match, _ := mergeTopologyMatchWithLimiter(base, other, nil)
	return match
}

func mergeTopologyMatchWithLimiter(base, other topologymodel.Match, limiter worklimit.Limiter) (topologymodel.Match, error) {
	var err error
	if base.ChassisIDs, err = appendUniqueTopologyStringsWithLimiter(limiter, base.ChassisIDs, other.ChassisIDs...); err != nil {
		return topologymodel.Match{}, err
	}
	if base.MacAddresses, err = appendUniqueTopologyStringsWithLimiter(limiter, base.MacAddresses, other.MacAddresses...); err != nil {
		return topologymodel.Match{}, err
	}
	if base.IPAddresses, err = appendUniqueTopologyStringsWithLimiter(limiter, base.IPAddresses, other.IPAddresses...); err != nil {
		return topologymodel.Match{}, err
	}
	if base.Hostnames, err = appendUniqueTopologyStringsWithLimiter(limiter, base.Hostnames, other.Hostnames...); err != nil {
		return topologymodel.Match{}, err
	}
	if base.DNSNames, err = appendUniqueTopologyStringsWithLimiter(limiter, base.DNSNames, other.DNSNames...); err != nil {
		return topologymodel.Match{}, err
	}
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base, nil
}

type topologyMatchCollapseLists struct {
	chassisIDs   []string
	macAddresses []string
	ipAddresses  []string
	hostnames    []string
	dnsNames     []string
}

func (a *topologyMatchCollapseLists) add(match topologymodel.Match) {
	_ = a.addWithLimiter(match, nil)
}

func (a *topologyMatchCollapseLists) addWithLimiter(match topologymodel.Match, limiter worklimit.Limiter) error {
	if err := topologymodel.ChargeMatch(limiter, match); err != nil {
		return err
	}
	a.chassisIDs = append(a.chassisIDs, match.ChassisIDs...)
	a.macAddresses = append(a.macAddresses, match.MacAddresses...)
	a.ipAddresses = append(a.ipAddresses, match.IPAddresses...)
	a.hostnames = append(a.hostnames, match.Hostnames...)
	a.dnsNames = append(a.dnsNames, match.DNSNames...)
	return nil
}

func (a *topologyMatchCollapseLists) apply(match *topologymodel.Match) {
	_ = a.applyWithLimiter(match, nil)
}

func (a *topologyMatchCollapseLists) applyWithLimiter(match *topologymodel.Match, limiter worklimit.Limiter) error {
	var err error
	if match.ChassisIDs, err = appendUniqueTopologyStringsWithLimiter(limiter, nil, a.chassisIDs...); err != nil {
		return err
	}
	if match.MacAddresses, err = appendUniqueTopologyStringsWithLimiter(limiter, nil, a.macAddresses...); err != nil {
		return err
	}
	if match.IPAddresses, err = appendUniqueTopologyStringsWithLimiter(limiter, nil, a.ipAddresses...); err != nil {
		return err
	}
	if match.Hostnames, err = appendUniqueTopologyStringsWithLimiter(limiter, nil, a.hostnames...); err != nil {
		return err
	}
	match.DNSNames, err = appendUniqueTopologyStringsWithLimiter(limiter, nil, a.dnsNames...)
	return err
}

func clearTopologyMatchCollapseLists(match *topologymodel.Match) {
	match.ChassisIDs = nil
	match.MacAddresses = nil
	match.IPAddresses = nil
	match.Hostnames = nil
	match.DNSNames = nil
}

func mergeTopologyStringMap(base, other map[string]string) map[string]string {
	merged, _ := mergeTopologyStringMapWithLimiter(base, other, nil)
	return merged
}

func mergeTopologyStringMapWithLimiter(
	base, other map[string]string,
	limiter worklimit.Limiter,
) (map[string]string, error) {
	if len(other) == 0 {
		return base, nil
	}
	if err := limiter.Charge(uint64(len(other))); err != nil {
		return nil, err
	}
	if base == nil {
		base = make(map[string]string, len(other))
	}
	for key, value := range other {
		if err := worklimit.ChargeStrings(limiter, []string{key, value}); err != nil {
			return nil, err
		}
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
	return base, nil
}

func appendUniqueTopologyStrings(base []string, values ...string) []string {
	out, _ := appendUniqueTopologyStringsWithLimiter(nil, base, values...)
	return out
}

func appendUniqueTopologyStringsWithLimiter(limiter worklimit.Limiter, base []string, values ...string) ([]string, error) {
	if err := worklimit.ChargeStrings(limiter, base); err != nil {
		return nil, err
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return nil, err
	}
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
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return nil, err
	}
	return out, nil
}
