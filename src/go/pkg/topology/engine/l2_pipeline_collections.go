// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"sort"
	"strings"
)

func sortedAddrValues(in map[string]netip.Addr) []netip.Addr {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]netip.Addr, 0, len(keys))
	for _, key := range keys {
		if addr, ok := in[key]; ok && addr.IsValid() {
			out = append(out, addr)
		}
	}
	return out
}

func setToCSV(in map[string]struct{}) string {
	if len(in) == 0 {
		return ""
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func csvToTopologySet(value string) map[string]struct{} {
	out := make(map[string]struct{})
	for token := range strings.SplitSeq(strings.TrimSpace(value), ",") {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func observationProtocolsUsed(obs L2Observation) map[string]struct{} {
	out := make(map[string]struct{}, 6)
	if len(obs.LLDPRemotes) > 0 {
		out["lldp"] = struct{}{}
	}
	if len(obs.CDPRemotes) > 0 {
		out["cdp"] = struct{}{}
	}
	if len(obs.BridgePorts) > 0 {
		out["bridge"] = struct{}{}
	}
	if len(obs.FDBEntries) > 0 {
		out["fdb"] = struct{}{}
	}
	if len(obs.STPPorts) > 0 {
		out["stp"] = struct{}{}
	}
	if len(obs.ARPNDEntries) > 0 {
		out["arp"] = struct{}{}
	}
	return out
}

func pruneEmptyLabels(labels map[string]string) {
	for key, value := range labels {
		if strings.TrimSpace(value) == "" {
			delete(labels, key)
		}
	}
}
