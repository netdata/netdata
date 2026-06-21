// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"sort"
	"strings"
)

func buildTopologyManagedFocusTargets(snapshots []topologymodel.ObservationSnapshot) []topologyoptions.ManagedFocusTarget {
	if len(snapshots) == 0 {
		return nil
	}

	targetByValue := make(map[string]topologyoptions.ManagedFocusTarget)
	for _, snapshot := range snapshots {
		managementIP := topologyutil.NormalizeIPAddress(snapshot.LocalDevice.ManagementIP)
		if managementIP == "" && len(snapshot.L2Observations) > 0 {
			managementIP = topologyutil.NormalizeIPAddress(snapshot.L2Observations[0].ManagementIP)
		}
		if managementIP == "" {
			continue
		}
		value := topologyoptions.ManagedFocusIPPrefix + managementIP

		displayName := strings.TrimSpace(snapshot.LocalDevice.SysName)
		if displayName == "" && len(snapshot.L2Observations) > 0 {
			displayName = strings.TrimSpace(snapshot.L2Observations[0].Hostname)
		}
		if displayName == "" {
			displayName = managementIP
		}
		label := displayName
		if !strings.EqualFold(displayName, managementIP) {
			label = displayName + " (" + managementIP + ")"
		}

		existing, exists := targetByValue[value]
		if !exists || label < existing.Name {
			targetByValue[value] = topologyoptions.ManagedFocusTarget{
				Value: value,
				Name:  label,
			}
		}
	}
	if len(targetByValue) == 0 {
		return nil
	}

	out := make([]topologyoptions.ManagedFocusTarget, 0, len(targetByValue))
	for _, target := range targetByValue {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(out[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(out[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return out[i].Value < out[j].Value
	})
	return out
}
