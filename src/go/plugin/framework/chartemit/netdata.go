// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

var wireValueReplacer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)

func sanitizeWireValue(value string) string {
	return wireValueReplacer.Replace(value)
}

func sanitizeWireID(value string) string {
	return sanitizeWireValue(strings.TrimSpace(value))
}

func emitChart(api *netdataapi.API, env EmitEnv, chartID string, meta chartengine.ChartMeta, obsolete bool) {
	opts := ""
	if obsolete {
		opts = "obsolete"
	}
	api.CHART(netdataapi.ChartOpts{
		TypeID:      sanitizeWireID(env.TypeID),
		ID:          sanitizeWireID(chartID),
		Name:        "",
		Title:       sanitizeWireValue(meta.Title),
		Units:       sanitizeWireValue(meta.Units),
		Family:      sanitizeWireValue(meta.Family),
		Context:     sanitizeWireValue(meta.Context),
		ChartType:   string(meta.Type),
		Priority:    meta.Priority,
		UpdateEvery: env.UpdateEvery,
		Options:     opts,
		Plugin:      sanitizeWireValue(env.Plugin),
		Module:      sanitizeWireValue(env.Module),
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
		sKey := sanitizeWireID(key)
		if sKey == "" {
			continue
		}
		api.CLABEL(sKey, sanitizeWireValue(env.JobLabels[key]), labelSourceConf)
	}
	for _, key := range chartKeys {
		sKey := sanitizeWireID(key)
		if sKey == "" {
			continue
		}
		api.CLABEL(sKey, sanitizeWireValue(chartLabels[key]), labelSourceAuto)
	}
	if strings.TrimSpace(env.JobName) != "" {
		api.CLABEL(collectJobReservedLabel, sanitizeWireValue(env.JobName), labelSourceAuto)
	}
}
