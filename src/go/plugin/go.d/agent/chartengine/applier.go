// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

const (
	labelSourceAuto = 1
	labelSourceConf = 2
)

// EmitEnv carries runtime context for translating engine actions to Netdata wire.
type EmitEnv struct {
	TypeID      string
	UpdateEvery int
	Plugin      string
	Module      string
	JobName     string
	JobLabels   map[string]string
	MSSinceLast int
}

// ApplyPlan emits chartengine actions to the Netdata wire API.
func ApplyPlan(api *netdataapi.API, plan Plan, env EmitEnv) error {
	if api == nil {
		return fmt.Errorf("chartengine: nil netdata api")
	}
	if strings.TrimSpace(env.TypeID) == "" {
		return fmt.Errorf("chartengine: emit env type_id is required")
	}
	if env.UpdateEvery <= 0 {
		env.UpdateEvery = 1
	}

	// 1) Create phase.
	for _, action := range plan.Actions {
		createChart, ok := action.(CreateChartAction)
		if !ok {
			continue
		}
		emitChart(api, env, createChart.ChartID, createChart.Meta, false)
		emitChartLabels(api, env)
		api.CLABELCOMMIT()
	}
	for _, action := range plan.Actions {
		createDim, ok := action.(CreateDimensionAction)
		if !ok {
			continue
		}
		emitChart(api, env, createDim.ChartID, createDim.ChartMeta, false)
		api.DIMENSION(netdataapi.DimensionOpts{
			ID:         createDim.Name,
			Name:       createDim.Name,
			Algorithm:  string(createDim.Algorithm),
			Multiplier: handleZero(createDim.Multiplier),
			Divisor:    handleZero(createDim.Divisor),
			Options:    makeDimensionOptions(createDim.Hidden, false),
		})
	}

	// 2) Update phase.
	for _, action := range plan.Actions {
		update, ok := action.(UpdateChartAction)
		if !ok {
			continue
		}
		api.BEGIN(env.TypeID, update.ChartID, env.MSSinceLast)
		for _, dim := range update.Values {
			if dim.IsEmpty {
				api.SETEMPTY(dim.Name)
				continue
			}
			if dim.IsFloat {
				api.SETFLOAT(dim.Name, dim.Float64)
				continue
			}
			api.SET(dim.Name, dim.Int64)
		}
		api.END()
	}

	// 3) Remove phase.
	for _, action := range plan.Actions {
		removeDim, ok := action.(RemoveDimensionAction)
		if !ok {
			continue
		}
		emitChart(api, env, removeDim.ChartID, removeDim.ChartMeta, false)
		api.DIMENSION(netdataapi.DimensionOpts{
			ID:         removeDim.Name,
			Name:       removeDim.Name,
			Algorithm:  string(removeDim.Algorithm),
			Multiplier: handleZero(removeDim.Multiplier),
			Divisor:    handleZero(removeDim.Divisor),
			Options:    makeDimensionOptions(removeDim.Hidden, true),
		})
	}
	for _, action := range plan.Actions {
		removeChart, ok := action.(RemoveChartAction)
		if !ok {
			continue
		}
		emitChart(api, env, removeChart.ChartID, removeChart.Meta, true)
	}

	return nil
}

func emitChart(api *netdataapi.API, env EmitEnv, chartID string, meta program.ChartMeta, obsolete bool) {
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

func emitChartLabels(api *netdataapi.API, env EmitEnv) {
	keys := make([]string, 0, len(env.JobLabels))
	for key := range env.JobLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		api.CLABEL(key, env.JobLabels[key], labelSourceConf)
	}
	if strings.TrimSpace(env.JobName) != "" {
		api.CLABEL("_collect_job", env.JobName, labelSourceAuto)
	}
}

func makeDimensionOptions(hidden, obsolete bool) string {
	var parts []string
	if hidden {
		parts = append(parts, "hidden")
	}
	if obsolete {
		parts = append(parts, "obsolete")
	}
	return strings.Join(parts, " ")
}

func handleZero(v int) int {
	if v == 0 {
		return 1
	}
	return v
}
