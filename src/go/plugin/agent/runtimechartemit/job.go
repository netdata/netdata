// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"maps"
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

type runtimePreparedEmission struct {
	attempt         chartengine.PlanAttempt
	nextPrev        time.Time
	nextKnownCharts map[string]chartengine.ChartMeta
}

type runtimeTickCommitStep struct {
	attempt  chartengine.PlanAttempt
	finalize func()
}

type runtimeTickCommit struct {
	steps []runtimeTickCommitStep
}

func (c *runtimeTickCommit) add(step runtimeTickCommitStep) {
	c.steps = append(c.steps, step)
}

func (c *runtimeTickCommit) abort() {
	for _, step := range c.steps {
		step.attempt.Abort()
	}
}

func (c *runtimeTickCommit) commit() error {
	for i, step := range c.steps {
		if err := step.attempt.Commit(); err != nil {
			for _, remaining := range c.steps[i+1:] {
				remaining.attempt.Abort()
			}
			return err
		}
		if step.finalize != nil {
			step.finalize()
		}
	}
	return nil
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
	var commit runtimeTickCommit

	for _, spec := range specs {
		seen[spec.Name] = struct{}{}

		if spec.UpdateEvery > 1 && clock%spec.UpdateEvery != 0 {
			continue
		}

		step, buf, ok := j.prepareComponentStep(spec, now)
		if !ok {
			continue
		}
		if buf != nil && buf.Len() > 0 {
			_, _ = j.buf.Write(buf.Bytes())
		}
		commit.add(step)
	}

	for name := range j.components {
		if _, ok := seen[name]; ok {
			continue
		}
		state := j.components[name]
		step, buf, ok := j.prepareRemovalStep(name, state)
		if !ok {
			continue
		}
		if buf != nil && buf.Len() > 0 {
			_, _ = j.buf.Write(buf.Bytes())
		}
		commit.add(step)
	}

	if j.buf.Len() > 0 {
		_, _ = io.Copy(j.out, j.buf)
	}
	if err := commit.commit(); err != nil {
		j.Warningf("runtime metrics commit failed: %v", err)
	}
	j.buf.Reset()
}

func (j *runtimeMetricsJob) newComponentState(spec componentSpec) (*runtimeComponentState, error) {
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
	return state, nil
}

func (j *runtimeMetricsJob) prepareComponentStep(spec componentSpec, now time.Time) (runtimeTickCommitStep, *bytes.Buffer, bool) {
	current := j.components[spec.Name]
	if current != nil && current.spec.Generation == spec.Generation {
		emission, buf, ok := j.prepareEmissionToBuffer(current, now)
		if !ok {
			return runtimeTickCommitStep{}, nil, false
		}
		return runtimeTickCommitStep{
			attempt: emission.attempt,
			finalize: func() {
				current.prev = emission.nextPrev
				current.knownCharts = emission.nextKnownCharts
			},
		}, buf, true
	}

	next, err := j.newComponentState(spec)
	if err != nil {
		j.Warningf("runtime metrics component %q init failed: %v", spec.Name, err)
		return runtimeTickCommitStep{}, nil, false
	}

	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	if err := j.emitComponentObsolete(api, current); err != nil {
		j.Warningf("runtime metrics component %q obsolete emit failed: %v", spec.Name, err)
		return runtimeTickCommitStep{}, nil, false
	}
	emission, ok := j.prepareEmission(api, next, now)
	if !ok {
		return runtimeTickCommitStep{}, nil, false
	}
	return runtimeTickCommitStep{
		attempt: emission.attempt,
		finalize: func() {
			next.prev = emission.nextPrev
			next.knownCharts = emission.nextKnownCharts
			j.components[spec.Name] = next
		},
	}, &buf, true
}

func (j *runtimeMetricsJob) prepareRemovalStep(name string, state *runtimeComponentState) (runtimeTickCommitStep, *bytes.Buffer, bool) {
	if state == nil {
		return runtimeTickCommitStep{}, nil, false
	}

	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	if err := j.emitComponentObsolete(api, state); err != nil {
		j.Warningf("runtime metrics component %q obsolete emit failed: %v", state.spec.Name, err)
		return runtimeTickCommitStep{}, nil, false
	}
	return runtimeTickCommitStep{
		finalize: func() {
			delete(j.components, name)
		},
	}, &buf, true
}

func (j *runtimeMetricsJob) prepareEmissionToBuffer(state *runtimeComponentState, now time.Time) (runtimePreparedEmission, *bytes.Buffer, bool) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	emission, ok := j.prepareEmission(api, state, now)
	if !ok {
		return runtimePreparedEmission{}, nil, false
	}
	return emission, &buf, true
}

func (j *runtimeMetricsJob) prepareEmission(api *netdataapi.API, state *runtimeComponentState, now time.Time) (runtimePreparedEmission, bool) {
	if state == nil {
		return runtimePreparedEmission{}, false
	}

	reader := state.spec.Store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	attempt, err := state.engine.PreparePlan(reader)
	if err != nil {
		j.Warningf("runtime metrics component %q build plan failed: %v", state.spec.Name, err)
		return runtimePreparedEmission{}, false
	}

	plan := attempt.Plan()
	env := cloneEmitEnv(state.spec.EmitEnv)
	env.MSSinceLast = calcRuntimeSinceLast(now, state.prev)
	if err := chartemit.ApplyPlan(api, plan, env); err != nil {
		attempt.Abort()
		j.Warningf("runtime metrics component %q apply plan failed: %v", state.spec.Name, err)
		return runtimePreparedEmission{}, false
	}

	return runtimePreparedEmission{
		attempt:         attempt,
		nextPrev:        now,
		nextKnownCharts: applyEffectiveChartSet(state.knownCharts, plan),
	}, true
}

func (j *runtimeMetricsJob) emitComponentObsolete(api *netdataapi.API, state *runtimeComponentState) error {
	if state == nil || len(state.knownCharts) == 0 {
		return nil
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
	return chartemit.ApplyPlan(api, chartengine.Plan{Actions: actions}, env)
}

func applyEffectiveChartSet(known map[string]chartengine.ChartMeta, plan chartengine.Plan) map[string]chartengine.ChartMeta {
	out := maps.Clone(known)
	if out == nil {
		out = make(map[string]chartengine.ChartMeta)
	}

	createCharts := make(map[string]chartengine.ChartMeta)
	dimensionOnlyCharts := make(map[string]chartengine.ChartMeta)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			createCharts[v.ChartID] = v.Meta
		case chartengine.CreateDimensionAction:
			if _, ok := createCharts[v.ChartID]; ok {
				continue
			}
			if _, ok := dimensionOnlyCharts[v.ChartID]; !ok {
				dimensionOnlyCharts[v.ChartID] = v.ChartMeta
			}
		case chartengine.RemoveChartAction:
			delete(out, v.ChartID)
		}
	}
	maps.Copy(out, createCharts)
	for chartID, meta := range dimensionOnlyCharts {
		if _, ok := out[chartID]; ok {
			continue
		}
		out[chartID] = meta
	}
	return out
}

func calcRuntimeSinceLast(cur, prev time.Time) int {
	if prev.IsZero() {
		return 0
	}
	// Keep parity with module.Job calcSinceLastRun() units.
	return int((cur.UnixNano() - prev.UnixNano()) / 1000)
}
