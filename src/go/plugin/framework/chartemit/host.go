// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"fmt"
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// PrepareHostInfo normalizes host-definition payloads before HOST_DEFINE.
//
// Current semantics intentionally match v1 vnode emission:
// - GUID/hostname must be present,
// - "_hostname" is injected when absent,
// - label values are sanitized for Netdata wire output.
func PrepareHostInfo(info netdataapi.HostInfo) (netdataapi.HostInfo, error) {
	guid := strings.TrimSpace(info.GUID)
	if guid == "" {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host guid is required")
	}
	hostname := strings.TrimSpace(info.Hostname)
	if hostname == "" {
		return netdataapi.HostInfo{}, fmt.Errorf("chartemit: host hostname is required")
	}

	labels := maps.Clone(info.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	if _, ok := labels["_hostname"]; !ok {
		labels["_hostname"] = hostname
	}
	for key, value := range labels {
		labels[key] = sanitizeWireValue(value)
	}

	return netdataapi.HostInfo{
		GUID:     guid,
		Hostname: hostname,
		Labels:   labels,
	}, nil
}
