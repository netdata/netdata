// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

func sanitizeWireID(value string) string {
	return netdataapi.SanitizeWireValue(strings.TrimSpace(value))
}

func emitChart(api *netdataapi.API, env EmitEnv, chartID string, meta chartengine.ChartMeta, obsolete bool) {
	api.CHART(prepareChart(env, chartID, meta, obsolete))
}

func prepareChart(env EmitEnv, chartID string, meta chartengine.ChartMeta, obsolete bool) netdataapi.ChartOpts {
	opts := ""
	if obsolete {
		opts = "obsolete"
		if env.StoreFirst {
			opts = "obsolete store_first"
		}
	} else if env.StoreFirst {
		opts = "store_first"
	}
	return netdataapi.ChartOpts{
		TypeID:      sanitizeWireID(env.TypeID),
		ID:          sanitizeWireID(chartID),
		Name:        "",
		Title:       netdataapi.SanitizeWireValue(meta.Title),
		Units:       netdataapi.SanitizeWireValue(meta.Units),
		Family:      netdataapi.SanitizeWireValue(meta.Family),
		Context:     netdataapi.SanitizeWireValue(meta.Context),
		ChartType:   string(meta.Type),
		Priority:    meta.Priority,
		UpdateEvery: env.UpdateEvery,
		Options:     opts,
		Plugin:      netdataapi.SanitizeWireValue(env.Plugin),
		Module:      netdataapi.SanitizeWireValue(env.Module),
	}
}

func emitChartLabels(api *netdataapi.API, env EmitEnv, chartLabels map[string]string) {
	chartKeys := make([]string, 0, len(chartLabels))
	for key := range chartLabels {
		if strings.TrimSpace(key) == "" || key == collectJobReservedLabel {
			continue
		}
		chartKeys = append(chartKeys, key)
	}
	sort.Strings(chartKeys)

	keys := make([]string, 0, len(env.JobLabels))
	for key := range env.JobLabels {
		if _, overridden := chartLabels[key]; overridden {
			continue
		}
		if key == collectJobReservedLabel {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sKey := sanitizeWireID(key)
		if sKey == "" {
			continue
		}
		api.CLABEL(sKey, netdataapi.SanitizeWireValue(env.JobLabels[key]), labelSourceConf)
	}
	for _, key := range chartKeys {
		sKey := sanitizeWireID(key)
		if sKey == "" {
			continue
		}
		api.CLABEL(sKey, netdataapi.SanitizeWireValue(chartLabels[key]), labelSourceAuto)
	}
	if strings.TrimSpace(env.JobName) != "" {
		api.CLABEL(collectJobReservedLabel, netdataapi.SanitizeWireValue(env.JobName), labelSourceAuto)
	}
}
