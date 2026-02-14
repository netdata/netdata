// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
)

func emitChart(api *netdataapi.API, env EmitEnv, chartID string, meta chartengine.ChartMeta, obsolete bool) {
	opts := ""
	if obsolete {
		opts = "obsolete"
	}
	api.CHART(netdataapi.ChartOpts{
		TypeID:      env.TypeID,
		ID:          chartID,
		Name:        "",
		Title:       meta.Title,
		Units:       meta.Units,
		Family:      meta.Family,
		Context:     meta.Context,
		ChartType:   string(meta.Type),
		Priority:    meta.Priority,
		UpdateEvery: env.UpdateEvery,
		Options:     opts,
		Plugin:      env.Plugin,
		Module:      env.Module,
	})
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
		api.CLABEL(key, env.JobLabels[key], labelSourceConf)
	}
	for _, key := range chartKeys {
		api.CLABEL(key, chartLabels[key], labelSourceAuto)
	}
	if strings.TrimSpace(env.JobName) != "" {
		api.CLABEL(collectJobReservedLabel, env.JobName, labelSourceAuto)
	}
}
