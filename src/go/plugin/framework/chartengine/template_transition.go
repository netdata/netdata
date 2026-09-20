// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// templateTransition contains only the program state admitted with an attempt.
// Materialized state is staged separately by the ordinary planner.
type templateTransition struct {
	set   *TemplateSet
	cache *routeCache
}

func (t *templateTransition) install(state *engineState) {
	state.templateSet = t.set
	state.program = t.set.program
	state.matchIndex = t.set.index
	state.routeCache = t.cache
	state.cfg.autogen = t.set.policy.autogen
	state.cfg.autogenRules = t.set.policy.autogenRules
	state.cfg.selector = t.set.policy.selector
	state.cfg.autogenContextNamespace = t.set.global.namespace
}

func (e *Engine) prepareTemplateTransition(opts PlanOptions) (*Engine, *templateTransition, map[string]*materializedChartState, error) {
	changed := opts.TemplateSet != nil && opts.TemplateSet != e.state.templateSet
	if !changed && !opts.ResetMaterialized {
		return e, nil, nil, nil
	}
	// Copy the state value, not the engine's mutex. Shared program data is immutable;
	// materialized state is copied by buildPlan before any mutation.
	view := &Engine{
		state: e.state,
	}
	var transition *templateTransition
	var retired map[string]*materializedChartState
	if changed {
		set := opts.TemplateSet
		if set.program == nil {
			return nil, nil, nil, fmt.Errorf("chartengine: unprepared template set")
		}
		if e.state.templateSet != nil && !set.SameGlobalPolicy(e.state.templateSet) {
			return nil, nil, nil, fmt.Errorf("chartengine: template set changes fixed global policy or fallback namespace")
		}
		transition = &templateTransition{
			set:   set,
			cache: newRouteCache(),
		}
		transition.install(&view.state)
		if !opts.ResetMaterialized {
			retained := make(map[string]*materializedChartState, len(e.state.materialized.charts))
			unchanged := make(map[string]bool, len(set.entries))
			for _, entry := range set.entries {
				unchanged[entry.ID] = set.preservesEntry(e.state.templateSet, entry.ID)
			}
			for id, chart := range e.state.materialized.charts {
				keep := isAutogenTemplateID(chart.templateID)
				if !keep && e.state.templateSet != nil {
					old := e.state.matchIndex.chartsByID[chart.templateID]
					entryID := old.EntryID
					if e.state.templateSet.legacy {
						entryID = "document"
					}
					keep = unchanged[entryID]
				}
				if keep {
					retained[id] = chart
				} else {
					if retired == nil {
						retired = make(map[string]*materializedChartState)
					}
					retired[id] = chart
				}
			}
			view.state.materialized = materializedState{
				charts: retained,
			}
		}
	}
	if opts.ResetMaterialized {
		view.state.materialized = newMaterializedState()
	}
	return view, transition, retired, nil
}

func (ctx *planBuildContext) rememberRetired(id string, chart *materializedChartState) {
	if ctx.retired == nil {
		ctx.retired = make(map[string]*materializedChartState)
	}
	if _, exists := ctx.retired[id]; !exists {
		ctx.retired[id] = chart
	}
}

// Reconcile against final survivors, including quiet charts. Retirements must
// never undo a chart or dimension created by a different owner in this frame.
func (ctx *planBuildContext) reconcileRetirements() {
	if len(ctx.retired) == 0 {
		return
	}
	// Existing caps/expiry may already describe these identities. Replace those
	// actions with one final comparison for each affected chart.
	actions := ctx.out.Actions[:0]
	for _, action := range ctx.out.Actions {
		id := ""
		switch a := action.(type) {
		case RemoveChartAction:
			id = a.ChartID
		case RemoveDimensionAction:
			id = a.ChartID
		}
		if _, affected := ctx.retired[id]; !affected {
			actions = append(actions, action)
		}
	}
	ctx.out.Actions = actions
	ids := make([]string, 0, len(ctx.retired))
	for id := range ctx.retired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		old := ctx.retired[id]
		final := ctx.materialized.charts[id]
		if final == nil {
			ctx.out.Actions = append(ctx.out.Actions, RemoveChartAction{
				ChartID: id,
				Meta:    old.meta,
			})
			continue
		}
		names := make([]string, 0)
		for name := range old.dimensions {
			if _, exists := final.dimensions[name]; !exists {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			dim := old.dimensions[name]
			ctx.out.Actions = append(ctx.out.Actions, retiredDimension(id, name, final.meta, dim))
		}
	}
}

func retiredDimension(id, name string, meta program.ChartMeta, dim *materializedDimensionState) RemoveDimensionAction {
	return RemoveDimensionAction{
		ChartID:    id,
		ChartMeta:  meta,
		Name:       name,
		Hidden:     dim.hidden,
		Float:      dim.float,
		Algorithm:  dim.algorithm,
		Multiplier: dim.multiplier,
		Divisor:    dim.divisor,
	}
}
