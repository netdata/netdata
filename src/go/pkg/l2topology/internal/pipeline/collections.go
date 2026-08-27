// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"net/netip"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

func sortedAddrValues(limiter worklimit.Limiter, in map[string]netip.Addr) ([]netip.Addr, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(in))
	seen := make(map[netip.Addr]struct{}, len(in))
	for _, addr := range in {
		addr = addr.Unmap()
		if !addr.IsValid() {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if err := worklimit.SortSlice(limiter, out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 }); err != nil {
		return nil, err
	}
	return out, nil
}

func setToCSV(limiter worklimit.Limiter, in map[string]struct{}) (string, error) {
	if len(in) == 0 {
		return "", nil
	}
	if err := limiter.Charge(uint64(len(in))); err != nil {
		return "", err
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
		return "", nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return "", err
	}
	if err := worklimit.ChargeStrings(limiter, out); err != nil {
		return "", err
	}
	return strings.Join(out, ","), nil
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

func observationProtocolsUsed(obs model.L2Observation) map[string]struct{} {
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
