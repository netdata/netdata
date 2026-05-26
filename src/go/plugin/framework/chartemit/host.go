// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// PrepareHostInfo normalizes host-definition payloads before HOST_DEFINE.
//
// Current semantics intentionally match v1 vnode emission:
// - GUID/hostname must be present and wire-safe,
// - "_hostname" is injected when absent,
// - label keys and values are normalized for Netdata wire output.
func PrepareHostInfo(info netdataapi.HostInfo) (netdataapi.HostInfo, error) {
	guid := strings.TrimSpace(info.GUID)
	if guid == "" {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host guid is required")
	}
	if sanitizeWireID(guid) != guid {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host guid contains unsupported characters")
	}
	hostname := strings.TrimSpace(info.Hostname)
	if hostname == "" {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host hostname is required")
	}
	if sanitizeWireValue(hostname) != hostname {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host hostname contains unsupported characters")
	}

	labels := normalizeHostInfoLabels(info.Labels)
	if _, ok := labels["_hostname"]; !ok {
		labels["_hostname"] = hostname
	}

	return netdataapi.HostInfo{
		GUID:     guid,
		Hostname: hostname,
		Labels:   labels,
	}, nil
}

func normalizeHostInfoLabels(in map[string]string) map[string]string {
	labels := maps.Clone(in)
	if len(labels) == 0 {
		return make(map[string]string)
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]string, len(labels))
	for _, key := range keys {
		sKey := sanitizeWireID(key)
		if sKey == "" {
			continue
		}
		out[sKey] = sanitizeWireValue(labels[key])
	}
	return out
}
