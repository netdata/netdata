// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"
)

func enrichLocalActorChartReferences(actor *topologyActor, interfaceCharts map[string]topologyInterfaceChartRef) {
	if actor == nil || len(interfaceCharts) == 0 {
		return
	}

	lookup := topologyInterfaceChartLookup(interfaceCharts)
	if len(lookup) == 0 {
		return
	}

	if statuses, ok := actor.Attributes["if_statuses"]; ok && statuses != nil {
		actor.Attributes["if_statuses"] = enrichTopologyInterfaceStatusesWithChartRefs(statuses, lookup)
	}
	if actor.Tables != nil {
		if portRows, ok := actor.Tables["ports"]; ok && len(portRows) > 0 {
			enrichTopologyTableRowsWithChartRefs(portRows, lookup)
		}
	}
}

func topologyInterfaceChartLookup(interfaceCharts map[string]topologyInterfaceChartRef) map[string]topologyInterfaceChartRef {
	lookup := make(map[string]topologyInterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = deduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	return lookup
}

func enrichTopologyInterfaceStatusesWithChartRefs(
	statuses any,
	lookup map[string]topologyInterfaceChartRef,
) any {
	if len(lookup) == 0 || statuses == nil {
		return statuses
	}

	switch typed := statuses.(type) {
	case []map[string]any:
		for _, status := range typed {
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
		}
		return typed
	case []any:
		for i := range typed {
			status, ok := typed[i].(map[string]any)
			if !ok || status == nil {
				continue
			}
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
			typed[i] = status
		}
		return typed
	default:
		return statuses
	}
}

func enrichTopologyTableRowsWithChartRefs(rows []map[string]any, lookup map[string]topologyInterfaceChartRef) {
	if len(lookup) == 0 || len(rows) == 0 {
		return
	}

	for _, row := range rows {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["name"])))
		if name == "" {
			continue
		}
		ref, ok := lookup[name]
		if !ok {
			continue
		}
		row["chart_id_suffix"] = ref.ChartIDSuffix
		if len(ref.AvailableMetrics) > 0 {
			row["available_metrics"] = ref.AvailableMetrics
		}
	}
}
