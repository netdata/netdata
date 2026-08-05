// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
)

func collectAndCommit(ctx context.Context, coll *promcollector.Collector) error {
	managed, ok := metrix.AsCycleManagedStore(coll.MetricStore())
	if !ok {
		return &collectorCycleError{stage: "store", err: fmt.Errorf("collector store does not expose cycle control")}
	}
	controller := managed.CycleController()
	controller.BeginCycle()
	if err := coll.Collect(ctx); err != nil {
		controller.AbortCycle()
		return &collectorCycleError{stage: "collect", err: err}
	}
	if err := controller.CommitCycleSuccess(); err != nil {
		return &collectorCycleError{stage: "commit", err: err}
	}
	return nil
}

type collectorCycleError struct {
	stage string
	err   error
}

func (e *collectorCycleError) Error() string { return fmt.Sprintf("%s: %v", e.stage, e.err) }
func (e *collectorCycleError) Unwrap() error { return e.err }

type routePlanResult struct {
	plan   chartengine.Plan
	routes *planRouteSummary
}

type routePlanError struct {
	stage string
	err   error
}

func (e *routePlanError) Error() string { return fmt.Sprintf("%s: %v", e.stage, e.err) }
func (e *routePlanError) Unwrap() error { return e.err }

func prepareRoutePlan(reader metrix.Reader, templateYAML, emitTypeID string) (routePlanResult, error) {
	routes := newPlanRouteSummary()
	engine, err := chartengine.New(
		chartengine.WithEmitTypeIDBudgetPrefix(emitTypeID),
		chartengine.WithRuntimeStore(nil),
		chartengine.WithPlanRouteDiagnosticObserver(routes.observe),
	)
	if err != nil {
		return routePlanResult{}, &routePlanError{stage: "init", err: err}
	}
	if err := engine.LoadYAML([]byte(templateYAML), 1); err != nil {
		return routePlanResult{}, &routePlanError{stage: "load", err: err}
	}
	attempt, err := engine.PreparePlan(reader)
	if err != nil {
		return routePlanResult{}, &routePlanError{stage: "plan", err: err}
	}
	plan := attempt.Plan()
	if err := attempt.Commit(); err != nil {
		return routePlanResult{}, &routePlanError{stage: "commit", err: err}
	}
	return routePlanResult{plan: plan, routes: routes}, nil
}
