// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/tickstate"
)

type runtimeComponentState struct {
	spec        componentSpec
	engine      *chartengine.Engine
	prev        time.Time
	knownCharts map[string]chartengine.ChartMeta
}

type runtimeMetricsJob struct {
	*logger.Logger

	out      io.Writer
	registry *componentRegistry

	running atomic.Bool
	stop    chan struct{}
	tick    chan int

	buf *bytes.Buffer
	api *netdataapi.API

	components  map[string]*runtimeComponentState
	skipTracker tickstate.SkipTracker
}

func newRuntimeMetricsJob(out io.Writer, reg *componentRegistry, log *logger.Logger) *runtimeMetricsJob {
	if out == nil {
		out = io.Discard
	}
	if log == nil {
		log = logger.New().With(slog.String("component", "runtime-metrics-job"))
	}
	var buf bytes.Buffer
	return &runtimeMetricsJob{
		Logger:     log,
		out:        out,
		registry:   reg,
		stop:       make(chan struct{}),
		tick:       make(chan int, 1),
		buf:        &buf,
		api:        netdataapi.New(&buf),
		components: make(map[string]*runtimeComponentState),
	}
}

func (j *runtimeMetricsJob) Tick(clock int) {
	select {
	case j.tick <- clock:
	default:
		skip := j.skipTracker.MarkSkipped()

		if skip.RunStarted.IsZero() {
			j.Warning("skipping runtime metrics tick: waiting for first run to start")
			return
		}
		if skip.Count == 1 {
			j.Warningf("skipping runtime metrics tick: previous run still in progress for %s", time.Since(skip.RunStarted))
			return
		}
		j.Debugf("skipping runtime metrics tick: previous run still in progress for %s (skipped %d ticks)", time.Since(skip.RunStarted), skip.Count)
	}
}

func (j *runtimeMetricsJob) Start() {
	j.running.Store(true)
	j.Info("runtime metrics job started")
	defer func() {
		j.running.Store(false)
		j.Info("runtime metrics job stopped")
	}()

LOOP:
	for {
		select {
		case <-j.stop:
			break LOOP
		case clock := <-j.tick:
			resume := j.skipTracker.MarkRunStart(time.Now())
			if resume.Skipped > 0 {
				if resume.RunStopped.IsZero() || resume.RunStarted.IsZero() {
					j.Infof("runtime metrics tick resumed (skipped %d ticks)", resume.Skipped)
				} else {
					j.Infof(
						"runtime metrics tick resumed after %s (skipped %d ticks)",
						resume.RunStopped.Sub(resume.RunStarted),
						resume.Skipped,
					)
				}
			}
			j.runOnce(clock)
			j.skipTracker.MarkRunStop(time.Now())
		}
	}
	j.stop <- struct{}{}
}

func (j *runtimeMetricsJob) Stop() {
	j.stop <- struct{}{}
	<-j.stop
}

func (j *runtimeMetricsJob) runOnce(clock int) {
	specs := j.registry.snapshot()
	seen := make(map[string]struct{}, len(specs))
	now := time.Now()

	for _, spec := range specs {
		seen[spec.Name] = struct{}{}

		if spec.UpdateEvery > 1 && clock%spec.UpdateEvery != 0 {
			continue
		}

		component, err := j.ensureComponent(spec)
		if err != nil {
			j.Warningf("runtime metrics component %q init failed: %v", spec.Name, err)
			continue
		}

		reader := spec.Store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
		plan, err := component.engine.BuildPlan(reader)
		if err != nil {
			j.Warningf("runtime metrics component %q build plan failed: %v", spec.Name, err)
			continue
		}
		if len(plan.Actions) == 0 {
			continue
		}

		env := cloneEmitEnv(spec.EmitEnv)
		env.MSSinceLast = calcRuntimeSinceLast(now, component.prev)
		component.prev = now

		if err := chartemit.ApplyPlan(j.api, plan, env); err != nil {
			j.Warningf("runtime metrics component %q apply plan failed: %v", spec.Name, err)
			continue
		}
		component.trackPlan(plan)
	}

	for name := range j.components {
		state, ok := j.components[name]
		if ok && state != nil {
			if _, exists := seen[name]; exists {
				continue
			}
			j.emitComponentObsolete(state)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		delete(j.components, name)
	}

	if j.buf.Len() > 0 {
		_, _ = io.Copy(j.out, j.buf)
		j.buf.Reset()
	}
}

func (j *runtimeMetricsJob) ensureComponent(spec componentSpec) (*runtimeComponentState, error) {
	current, ok := j.components[spec.Name]
	if ok && current.spec.Generation == spec.Generation {
		return current, nil
	}

	engineLog := j.Logger.With(slog.String("runtime_component", spec.Name))
	engine, err := chartengine.New(
		chartengine.WithRuntimeStore(nil), // Two-engine policy: observer engine has no self-metrics.
		chartengine.WithSeriesSelectionAllVisible(),
		chartengine.WithRuntimePlannerMode(),
		chartengine.WithEmitTypeIDBudgetPrefix(spec.EmitEnv.TypeID),
		chartengine.WithEnginePolicy(chartengine.EnginePolicy{Autogen: &spec.Autogen}),
		chartengine.WithLogger(engineLog),
	)
	if err != nil {
		return nil, fmt.Errorf("create engine: %w", err)
	}
	if err := engine.LoadYAML(spec.TemplateYAML, spec.Generation); err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}

	state := &runtimeComponentState{
		spec:        spec,
		engine:      engine,
		knownCharts: make(map[string]chartengine.ChartMeta),
	}
	if ok && current != nil && current.spec.Generation != spec.Generation {
		j.emitComponentObsolete(current)
	}
	j.components[spec.Name] = state
	return state, nil
}

func (j *runtimeMetricsJob) emitComponentObsolete(state *runtimeComponentState) {
	if state == nil || len(state.knownCharts) == 0 {
		return
	}

	chartIDs := make([]string, 0, len(state.knownCharts))
	for chartID := range state.knownCharts {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	actions := make([]chartengine.EngineAction, 0, len(chartIDs))
	for _, chartID := range chartIDs {
		meta := state.knownCharts[chartID]
		actions = append(actions, chartengine.RemoveChartAction{
			ChartID: chartID,
			Meta:    meta,
		})
	}

	env := cloneEmitEnv(state.spec.EmitEnv)
	env.MSSinceLast = 0
	if err := chartemit.ApplyPlan(j.api, chartengine.Plan{Actions: actions}, env); err != nil {
		j.Warningf("runtime metrics component %q obsolete emit failed: %v", state.spec.Name, err)
		return
	}
	clear(state.knownCharts)
}

func (s *runtimeComponentState) trackPlan(plan chartengine.Plan) {
	if s == nil {
		return
	}
	if s.knownCharts == nil {
		s.knownCharts = make(map[string]chartengine.ChartMeta)
	}
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			s.knownCharts[v.ChartID] = v.Meta
		case chartengine.RemoveChartAction:
			delete(s.knownCharts, v.ChartID)
		}
	}
}

func calcRuntimeSinceLast(cur, prev time.Time) int {
	if prev.IsZero() {
		return 0
	}
	// Keep parity with module.Job calcSinceLastRun() units.
	return int((cur.UnixNano() - prev.UnixNano()) / 1000)
}
