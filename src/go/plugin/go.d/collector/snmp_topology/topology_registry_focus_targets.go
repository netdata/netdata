// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

func buildTopologyManagedFocusTargets(snapshots []topologyObservationSnapshot) []topologyManagedFocusTarget {
	if len(snapshots) == 0 {
		return nil
	}

	targetByValue := make(map[string]topologyManagedFocusTarget)
	for _, snapshot := range snapshots {
		managementIP := normalizeIPAddress(snapshot.localDevice.ManagementIP)
		if managementIP == "" && len(snapshot.l2Observations) > 0 {
			managementIP = normalizeIPAddress(snapshot.l2Observations[0].ManagementIP)
		}
		if managementIP == "" {
			continue
		}
		value := topologyManagedFocusIPPrefix + managementIP

		displayName := strings.TrimSpace(snapshot.localDevice.SysName)
		if displayName == "" && len(snapshot.l2Observations) > 0 {
			displayName = strings.TrimSpace(snapshot.l2Observations[0].Hostname)
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
			targetByValue[value] = topologyManagedFocusTarget{
				Value: value,
				Name:  label,
			}
		}
	}
	if len(targetByValue) == 0 {
		return nil
	}

	out := make([]topologyManagedFocusTarget, 0, len(targetByValue))
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
