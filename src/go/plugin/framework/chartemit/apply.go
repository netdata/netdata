// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// ErrTypeIDBudgetExceeded identifies an emitted type/chart ID that cannot fit
// the Netdata wire-protocol limit.
var ErrTypeIDBudgetExceeded = errors.New("chartemit: type.id exceeds max length")

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
	updateLabels     []UpdateChartLabelsAction
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

type definitionVisitor interface {
	visitChart(chartID string, meta chartengine.ChartMeta, labels map[string]string, emitLabels, obsolete bool)
	visitDimension(chartID string, dim dimensionEmission)
}

type emissionDefinitionVisitor struct {
	api *netdataapi.API
	env EmitEnv
}

func (v emissionDefinitionVisitor) visitChart(
	chartID string,
	meta chartengine.ChartMeta,
	labels map[string]string,
	emitLabels bool,
	obsolete bool,
) {
	emitChart(v.api, v.env, chartID, meta, obsolete)
	if emitLabels {
		emitChartLabels(v.api, v.env, labels)
		v.api.CLABELCOMMIT()
	}
}

func (v emissionDefinitionVisitor) visitDimension(_ string, dim dimensionEmission) {
	emitDimension(v.api, dim)
}

// ApplyPlan emits chartengine actions to the Netdata wire API.
func ApplyPlan(api *netdataapi.API, plan Plan, env EmitEnv) error {
	if api == nil {
		return fmt.Errorf("chartemit: nil netdata api")
	}
	if err := validateEmitEnv(env); err != nil {
		return err
	}
	if env.UpdateEvery <= 0 {
		env.UpdateEvery = 1
	}
	normalized := normalizeActions(plan.Actions)
	if err := validateTypeIDBudget(env.TypeID, normalized); err != nil {
		return err
	}
	if !hasEmissions(normalized) {
		return nil
	}
	if err := emitHostSelection(api, env); err != nil {
		return err
	}
	visitor := emissionDefinitionVisitor{api: api, env: env}
	visitCreatePhase(visitor, normalized)
	visitLabelUpdatePhase(visitor, normalized.updateLabels)
	emitUpdatePhase(api, env, normalized.updateCharts)
	visitRemovePhase(visitor, normalized)
	return nil
}

func validateEmitEnv(env EmitEnv) error {
	if strings.TrimSpace(env.TypeID) == "" {
		return fmt.Errorf("chartemit: emit env type_id is required")
	}
	return nil
}

func hasEmissions(actions normalizedActions) bool {
	return len(actions.createCharts) > 0 ||
		len(actions.createDimsByID) > 0 ||
		len(actions.updateLabels) > 0 ||
		len(actions.updateCharts) > 0 ||
		len(actions.removeDimensions) > 0 ||
		len(actions.removeCharts) > 0
}

func emitHostSelection(api *netdataapi.API, env EmitEnv) error {
	if env.HostScope == nil {
		api.HOST("")
		return nil
	}

	guid := strings.TrimSpace(env.HostScope.GUID)
	if guid == "" {
		return fmt.Errorf("chartemit: emit env host scope guid is required")
	}
	if sanitizeWireID(guid) != guid {
		return fmt.Errorf("chartemit: emit env host scope guid contains unsupported characters")
	}
	if env.HostScope.Define != nil {
		define, err := PrepareHostInfo(*env.HostScope.Define)
		if err != nil {
			return err
		}
		if define.GUID != guid {
			return fmt.Errorf("chartemit: host define guid %q does not match host scope guid %q", env.HostScope.Define.GUID, env.HostScope.GUID)
		}
		api.HOSTINFO(define)
	}
	api.HOST(guid)
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
	for _, update := range actions.updateLabels {
		seen[update.ChartID] = struct{}{}
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
			return fmt.Errorf("%w (%d): %s.%s", ErrTypeIDBudgetExceeded, maxTypeIDLen, typeID, id)
		}
	}
	return nil
}

func normalizeActions(actions []EngineAction) normalizedActions {
	var out normalizedActions
	for _, action := range actions {
		switch v := action.(type) {
		case CreateChartAction:
			if out.createCharts == nil {
				out.createCharts = make(map[string]CreateChartAction)
			}
			out.createCharts[v.ChartID] = v
		case CreateDimensionAction:
			if out.createDimsByID == nil {
				out.createDimsByID = make(map[string][]CreateDimensionAction)
			}
			out.createDimsByID[v.ChartID] = append(out.createDimsByID[v.ChartID], v)
		case UpdateChartLabelsAction:
			out.updateLabels = append(out.updateLabels, v)
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

func visitCreatePhase[V definitionVisitor](visitor V, actions normalizedActions) {
	createdChartIDs := make([]string, 0, len(actions.createCharts))
	for chartID := range actions.createCharts {
		createdChartIDs = append(createdChartIDs, chartID)
	}
	sort.Strings(createdChartIDs)
	for _, chartID := range createdChartIDs {
		createChart := actions.createCharts[chartID]
		visitor.visitChart(createChart.ChartID, createChart.Meta, createChart.Labels, true, false)
		dims := actions.createDimsByID[chartID]
		for _, dim := range dims {
			visitor.visitDimension(chartID, dimensionEmission{
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
		visitor.visitChart(chartID, dims[0].ChartMeta, nil, false, false)
		for _, dim := range dims {
			visitor.visitDimension(chartID, dimensionEmission{
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

func visitLabelUpdatePhase[V definitionVisitor](visitor V, updates []UpdateChartLabelsAction) {
	if len(updates) == 0 {
		return
	}
	if len(updates) > 1 {
		slices.SortFunc(updates, func(a, b UpdateChartLabelsAction) int {
			return cmp.Compare(a.ChartID, b.ChartID)
		})
	}
	for _, update := range updates {
		visitor.visitChart(update.ChartID, update.Meta, update.Labels, true, false)
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
				// Defensive: a non-finite float renders as 0 on the wire (the C parser accepts
				// only lowercase "nan"); emit a gap. The planner already maps these to IsEmpty.
				if math.IsNaN(dim.Float64) || math.IsInf(dim.Float64, 0) {
					api.SETEMPTY(sanitizeWireID(dim.Name))
					continue
				}
				api.SETFLOAT(sanitizeWireID(dim.Name), dim.Float64)
				continue
			}
			api.SET(sanitizeWireID(dim.Name), dim.Int64)
		}
		api.END()
	}
}

func visitRemovePhase[V definitionVisitor](visitor V, actions normalizedActions) {
	for _, removeDim := range actions.removeDimensions {
		visitor.visitChart(removeDim.ChartID, removeDim.ChartMeta, nil, false, false)
		visitor.visitDimension(removeDim.ChartID, dimensionEmission{
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
		visitor.visitChart(removeChart.ChartID, removeChart.Meta, nil, false, true)
	}
}

func emitDimension(api *netdataapi.API, dim dimensionEmission) {
	opts, ok := prepareDimension(dim)
	if !ok {
		return
	}
	api.DIMENSION(opts)
}

func prepareDimension(dim dimensionEmission) (netdataapi.DimensionOpts, bool) {
	name := sanitizeWireID(dim.Name)
	if name == "" {
		return netdataapi.DimensionOpts{}, false
	}
	return netdataapi.DimensionOpts{
		ID:         name,
		Name:       name,
		Algorithm:  dim.Algorithm,
		Multiplier: handleZero(dim.Multiplier),
		Divisor:    handleZero(dim.Divisor),
		Options:    makeDimensionOptions(dim.Hidden, dim.Obsolete, dim.Float),
	}, true
}
