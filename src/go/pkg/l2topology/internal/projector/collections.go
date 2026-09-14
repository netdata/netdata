// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"maps"
	"net/netip"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func addressStrings(addresses []netip.Addr) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.Unmap().String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func deviceAddressValues(dev model.Device) []netip.Addr {
	addresses := make([]netip.Addr, 0, len(dev.Addresses)+1)
	if dev.ManagementIP.IsValid() {
		addresses = append(addresses, dev.ManagementIP)
	}
	addresses = append(addresses, dev.Addresses...)
	return addresses
}

func deviceAddressStrings(dev model.Device) []string {
	return addressStrings(deviceAddressValues(dev))
}

func selectedDeviceManagementIP(dev model.Device) string {
	if addr := dev.ManagementIP.Unmap(); addr.IsValid() {
		return addr.String()
	}
	if len(dev.Addresses) == 1 {
		if addr := dev.Addresses[0].Unmap(); addr.IsValid() {
			return addr.String()
		}
	}
	return ""
}

func uniqueTopologyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedEndpointIPs(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		addr, ok := in[key]
		if !ok || !addr.IsValid() {
			continue
		}
		out = append(out, addr.Unmap().String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedTopologySet(in map[string]struct{}) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func csvToSet(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func labelsCSVToSlice(labels map[string]string, key string) []string {
	if len(labels) == 0 {
		return nil
	}
	return csvToSet(labels[key])
}
