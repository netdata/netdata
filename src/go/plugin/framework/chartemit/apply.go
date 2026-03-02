// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

const (
	// Netdata wire protocol label sources (CLABEL third argument).
	labelSourceAuto         = 1
	labelSourceConf         = 2
	collectJobReservedLabel = "_collect_job"
	maxTypeIDLen            = 1200
)

type normalizedActions struct {
	createCharts     map[string]CreateChartAction
	createDimsByID   map[string][]CreateDimensionAction
	updateCharts     []UpdateChartAction
	removeDimensions []RemoveDimensionAction
	removeCharts     []RemoveChartAction
}

type dimensionEmission struct {
	Name       string
	Hidden     bool
	Float      bool
	Algorithm  string
	Multiplier int
	Divisor    int
	Obsolete   bool
}

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
	normalized := normalizeActions(plan.Actions)
	if err := validateTypeIDBudget(env.TypeID, normalized); err != nil {
		return err
	}
	emitCreatePhase(api, env, normalized)
	emitUpdatePhase(api, env, normalized.updateCharts)
	emitRemovePhase(api, env, normalized)
	return nil
}

func validateTypeIDBudget(typeID string, actions normalizedActions) error {
	typeID = sanitizeWireID(typeID)
	if typeID == "" {
		return fmt.Errorf("chartemit: emit env type_id is required")
	}
	seen := make(map[string]struct{})
	for chartID := range actions.createCharts {
		seen[chartID] = struct{}{}
	}
	for chartID := range actions.createDimsByID {
		seen[chartID] = struct{}{}
	}
	for _, update := range actions.updateCharts {
		seen[update.ChartID] = struct{}{}
	}
	for _, removeDim := range actions.removeDimensions {
		seen[removeDim.ChartID] = struct{}{}
	}
	for _, removeChart := range actions.removeCharts {
		seen[removeChart.ChartID] = struct{}{}
	}
	for chartID := range seen {
		id := sanitizeWireID(chartID)
		if len(typeID)+1+len(id) > maxTypeIDLen {
			return fmt.Errorf("chartemit: type.id exceeds max length (%d): %s.%s", maxTypeIDLen, typeID, id)
		}
	}
	return nil
}

func normalizeActions(actions []EngineAction) normalizedActions {
	out := normalizedActions{
		createCharts:   make(map[string]CreateChartAction),
		createDimsByID: make(map[string][]CreateDimensionAction),
	}
	for _, action := range actions {
		switch v := action.(type) {
		case CreateChartAction:
			out.createCharts[v.ChartID] = v
		case CreateDimensionAction:
			out.createDimsByID[v.ChartID] = append(out.createDimsByID[v.ChartID], v)
		case UpdateChartAction:
			out.updateCharts = append(out.updateCharts, v)
		case RemoveDimensionAction:
			out.removeDimensions = append(out.removeDimensions, v)
		case RemoveChartAction:
			out.removeCharts = append(out.removeCharts, v)
		}
	}
	return out
}

func emitCreatePhase(api *netdataapi.API, env EmitEnv, actions normalizedActions) {
	createdChartIDs := make([]string, 0, len(actions.createCharts))
	for chartID := range actions.createCharts {
		createdChartIDs = append(createdChartIDs, chartID)
	}
	sort.Strings(createdChartIDs)
	for _, chartID := range createdChartIDs {
		createChart := actions.createCharts[chartID]
		emitChart(api, env, createChart.ChartID, createChart.Meta, false)
		emitChartLabels(api, env, createChart.Labels)
		api.CLABELCOMMIT()
		dims := actions.createDimsByID[chartID]
		for _, dim := range dims {
			emitDimension(api, dimensionEmission{
				Name:       dim.Name,
				Hidden:     dim.Hidden,
				Float:      dim.Float,
				Algorithm:  string(dim.Algorithm),
				Multiplier: dim.Multiplier,
				Divisor:    dim.Divisor,
			})
		}
		// Mark dimensions as emitted alongside explicit chart-create path.
		delete(actions.createDimsByID, chartID)
	}

	remainingChartIDs := make([]string, 0, len(actions.createDimsByID))
	for chartID := range actions.createDimsByID {
		remainingChartIDs = append(remainingChartIDs, chartID)
	}
	sort.Strings(remainingChartIDs)
	for _, chartID := range remainingChartIDs {
		dims := actions.createDimsByID[chartID]
		if len(dims) == 0 {
			continue
		}
		emitChart(api, env, chartID, dims[0].ChartMeta, false)
		// Dimension-only chart creation path still needs chart labels and commit.
		emitChartLabels(api, env, nil)
		api.CLABELCOMMIT()
		for _, dim := range dims {
			emitDimension(api, dimensionEmission{
				Name:       dim.Name,
				Hidden:     dim.Hidden,
				Float:      dim.Float,
				Algorithm:  string(dim.Algorithm),
				Multiplier: dim.Multiplier,
				Divisor:    dim.Divisor,
			})
		}
	}
}

func emitUpdatePhase(api *netdataapi.API, env EmitEnv, updates []UpdateChartAction) {
	for _, update := range updates {
		api.BEGIN(sanitizeWireID(env.TypeID), sanitizeWireID(update.ChartID), env.MSSinceLast)
		for _, dim := range update.Values {
			if dim.IsEmpty {
				api.SETEMPTY(sanitizeWireID(dim.Name))
				continue
			}
			if dim.IsFloat {
				api.SETFLOAT(sanitizeWireID(dim.Name), dim.Float64)
				continue
			}
			api.SET(sanitizeWireID(dim.Name), dim.Int64)
		}
		api.END()
	}
}

func emitRemovePhase(api *netdataapi.API, env EmitEnv, actions normalizedActions) {
	for _, removeDim := range actions.removeDimensions {
		emitChart(api, env, removeDim.ChartID, removeDim.ChartMeta, false)
		emitDimension(api, dimensionEmission{
			Name:       removeDim.Name,
			Hidden:     removeDim.Hidden,
			Float:      removeDim.Float,
			Algorithm:  string(removeDim.Algorithm),
			Multiplier: removeDim.Multiplier,
			Divisor:    removeDim.Divisor,
			Obsolete:   true,
		})
	}
	for _, removeChart := range actions.removeCharts {
		emitChart(api, env, removeChart.ChartID, removeChart.Meta, true)
	}
}

func emitDimension(api *netdataapi.API, dim dimensionEmission) {
	name := sanitizeWireID(dim.Name)
	if name == "" {
		return
	}
	api.DIMENSION(netdataapi.DimensionOpts{
		ID:         name,
		Name:       name,
		Algorithm:  dim.Algorithm,
		Multiplier: handleZero(dim.Multiplier),
		Divisor:    handleZero(dim.Divisor),
		Options:    makeDimensionOptions(dim.Hidden, dim.Obsolete, dim.Float),
	})
}
