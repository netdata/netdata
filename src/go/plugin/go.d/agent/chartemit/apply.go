// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

const (
	labelSourceAuto         = 1
	labelSourceConf         = 2
	collectJobReservedLabel = "_collect_job"
)

// ApplyPlan emits chartengine actions to the Netdata wire API.
func ApplyPlan(api *netdataapi.API, plan Plan, env EmitEnv) error {
	if api == nil {
		return fmt.Errorf("chartemit: nil netdata api")
	}
	if strings.TrimSpace(env.TypeID) == "" {
		return fmt.Errorf("chartemit: emit env type_id is required")
	}
	if env.UpdateEvery <= 0 {
		env.UpdateEvery = 1
	}

	// 1) Create phase.
	createCharts := make(map[string]CreateChartAction)
	createDimsByChart := make(map[string][]CreateDimensionAction)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case CreateChartAction:
			createCharts[v.ChartID] = v
		case CreateDimensionAction:
			createDimsByChart[v.ChartID] = append(createDimsByChart[v.ChartID], v)
		}
	}

	createdChartIDs := make([]string, 0, len(createCharts))
	for chartID := range createCharts {
		createdChartIDs = append(createdChartIDs, chartID)
	}
	sort.Strings(createdChartIDs)
	for _, chartID := range createdChartIDs {
		createChart := createCharts[chartID]
		emitChart(api, env, createChart.ChartID, createChart.Meta, false)
		emitChartLabels(api, env, createChart.Labels)
		api.CLABELCOMMIT()
		dims := createDimsByChart[chartID]
		for _, dim := range dims {
			api.DIMENSION(netdataapi.DimensionOpts{
				ID:         dim.Name,
				Name:       dim.Name,
				Algorithm:  string(dim.Algorithm),
				Multiplier: handleZero(dim.Multiplier),
				Divisor:    handleZero(dim.Divisor),
				Options:    makeDimensionOptions(dim.Hidden, false),
			})
		}
		delete(createDimsByChart, chartID)
	}

	remainingChartIDs := make([]string, 0, len(createDimsByChart))
	for chartID := range createDimsByChart {
		remainingChartIDs = append(remainingChartIDs, chartID)
	}
	sort.Strings(remainingChartIDs)
	for _, chartID := range remainingChartIDs {
		dims := createDimsByChart[chartID]
		if len(dims) == 0 {
			continue
		}
		emitChart(api, env, chartID, dims[0].ChartMeta, false)
		for _, dim := range dims {
			api.DIMENSION(netdataapi.DimensionOpts{
				ID:         dim.Name,
				Name:       dim.Name,
				Algorithm:  string(dim.Algorithm),
				Multiplier: handleZero(dim.Multiplier),
				Divisor:    handleZero(dim.Divisor),
				Options:    makeDimensionOptions(dim.Hidden, false),
			})
		}
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
