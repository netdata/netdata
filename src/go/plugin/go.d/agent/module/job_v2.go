// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
)

type JobV2Config struct {
	PluginName      string
	Name            string
	ModuleName      string
	FullName        string
	Module          ModuleV2
	Labels          map[string]string
	Out             io.Writer
	UpdateEvery     int
	AutoDetectEvery int
	Vnode           vnodes.VirtualNode
	FunctionOnly    bool
}

func NewJobV2(cfg JobV2Config) *JobV2 {
	var buf bytes.Buffer
	if cfg.UpdateEvery <= 0 {
		cfg.UpdateEvery = 1
	}

	j := &JobV2{
		pluginName:      cfg.PluginName,
		name:            cfg.Name,
		moduleName:      cfg.ModuleName,
		fullName:        cfg.FullName,
		updateEvery:     cfg.UpdateEvery,
		autoDetectEvery: cfg.AutoDetectEvery,
		functionOnly:    cfg.FunctionOnly,
		module:          cfg.Module,
		labels:          cloneLabels(cfg.Labels),
		out:             cfg.Out,
		stop:            make(chan struct{}),
		tick:            make(chan int),
		buf:             &buf,
		api:             netdataapi.New(&buf),
		vnode:           cfg.Vnode,
	}
	if j.out == nil {
		j.out = io.Discard
	}

	log := logger.New().With(
		slog.String("collector", j.ModuleName()),
		slog.String("job", j.Name()),
	)
	j.Logger = log
	if j.module != nil {
		j.module.GetBase().Logger = log
		if vnode := j.module.VirtualNode(); vnode != nil {
			*vnode = *cfg.Vnode.Copy()
		}
	}
	return j
}

type JobV2 struct {
	pluginName      string
	name            string
	moduleName      string
	fullName        string
	updateEvery     int
	autoDetectEvery int
	functionOnly    bool
	labels          map[string]string

	*logger.Logger

	module ModuleV2

	running atomic.Bool

	initialized bool
	panicked    bool

	store  metrix.CollectorStore
	cycle  metrix.CycleController
	engine *chartengine.Engine

	prevRun time.Time
	retries int

	vnodeMu sync.RWMutex
	vnode   vnodes.VirtualNode

	tick chan int
	out  io.Writer
	buf  *bytes.Buffer
	api  *netdataapi.API

	stop chan struct{}
}

func (j *JobV2) FullName() string   { return j.fullName }
func (j *JobV2) ModuleName() string { return j.moduleName }
func (j *JobV2) Name() string       { return j.name }
func (j *JobV2) Panicked() bool     { return j.panicked }
func (j *JobV2) IsRunning() bool    { return j.running.Load() }
func (j *JobV2) Module() ModuleV2   { return j.module }
func (j *JobV2) Collector() any     { return j.module }
func (j *JobV2) AutoDetectionEvery() int {
	return j.autoDetectEvery
}
func (j *JobV2) RetryAutoDetection() bool {
	return j.autoDetectEvery > 0
}
func (j *JobV2) Configuration() any {
	if j.module == nil {
		return nil
	}
	return j.module.Configuration()
}
func (j *JobV2) IsFunctionOnly() bool { return j.functionOnly }
func (j *JobV2) Vnode() vnodes.VirtualNode {
	j.vnodeMu.RLock()
	defer j.vnodeMu.RUnlock()
	return *j.vnode.Copy()
}
func (j *JobV2) UpdateVnode(vnode *vnodes.VirtualNode) {
	if vnode == nil {
		return
	}
	j.vnodeMu.Lock()
	j.vnode = *vnode.Copy()
	j.vnodeMu.Unlock()
	if j.module != nil {
		if mv := j.module.VirtualNode(); mv != nil {
			*mv = *vnode.Copy()
		}
	}
}
func (j *JobV2) Cleanup() {
	if j.module != nil {
		j.module.Cleanup(context.TODO())
	}
}

func (j *JobV2) AutoDetection() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic %v", r)
			j.panicked = true
			j.Errorf("PANIC %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
		if err != nil && j.module != nil {
			j.module.Cleanup(context.TODO())
		}
	}()

	if err = j.init(); err != nil {
		j.Errorf("init failed: %v", err)
		return err
	}
	if err = j.check(); err != nil {
		j.Errorf("check failed: %v", err)
		return err
	}
	if err = j.postCheck(); err != nil {
		j.Errorf("postCheck failed: %v", err)
		return err
	}
	return nil
}

func (j *JobV2) Start() {
	j.running.Store(true)
	j.Infof("started (v2), data collection interval %ds", j.updateEvery)
	defer func() {
		j.running.Store(false)
		j.Info("stopped")
	}()

LOOP:
	for {
		select {
		case <-j.stop:
			break LOOP
		case t := <-j.tick:
			if j.shouldCollect(t) {
				j.runOnce()
			}
		}
	}
	j.Cleanup()
	j.stop <- struct{}{}
}

func (j *JobV2) Stop() {
	j.stop <- struct{}{}
	<-j.stop
}

func (j *JobV2) Tick(clock int) {
	select {
	case j.tick <- clock:
	default:
	}
}

func (j *JobV2) shouldCollect(clock int) bool {
	return clock%(j.updateEvery+j.penalty()) == 0
}

func (j *JobV2) init() error {
	if j.initialized {
		return nil
	}
	if err := j.module.Init(context.TODO()); err != nil {
		return err
	}
	j.initialized = true
	return nil
}

func (j *JobV2) check() error {
	return j.module.Check(context.TODO())
}

func (j *JobV2) postCheck() error {
	store := j.module.MetricStore()
	if store == nil {
		return fmt.Errorf("nil metric store")
	}
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		return fmt.Errorf("metric store is not cycle-managed")
	}

	opts := []chartengine.Option{
		chartengine.WithLogger(j.Logger.With(slog.String("component", "chartengine"))),
	}
	if v, ok := j.module.(ModuleV2Autogen); ok {
		opts = append(opts, chartengine.WithAutogenPolicy(v.AutogenPolicy()))
	}

	engine, err := chartengine.New(opts...)
	if err != nil {
		return err
	}
	if err := engine.LoadYAML(j.module.ChartTemplateYAML(), 1); err != nil {
		return err
	}

	j.store = store
	j.cycle = managed.CycleController()
	j.engine = engine
	return nil
}

func (j *JobV2) runOnce() {
	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.prevRun = curTime

	if j.collectAndEmit(sinceLastRun) {
		j.retries = 0
	} else {
		j.retries++
	}
	_, _ = io.Copy(j.out, j.buf)
	j.buf.Reset()
}

func (j *JobV2) collectAndEmit(sinceLastRun int) bool {
	j.panicked = false
	cycleOpen := false

	defer func() {
		if r := recover(); r != nil {
			if cycleOpen {
				// Recover path must close staged frame to keep subsequent cycles valid.
				func() {
					defer func() { _ = recover() }()
					j.cycle.AbortCycle()
				}()
			}
			j.panicked = true
			j.Errorf("PANIC: %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	j.cycle.BeginCycle()
	cycleOpen = true
	if err := j.module.Collect(context.TODO()); err != nil {
		j.cycle.AbortCycle()
		cycleOpen = false
		j.Warningf("collect failed: %v", err)
		return false
	}
	j.cycle.CommitCycleSuccess()
	cycleOpen = false

	plan, err := j.engine.BuildPlan(j.store.ReadRaw())
	if err != nil {
		j.Warningf("build plan failed: %v", err)
		return false
	}

	if err := chartemit.ApplyPlan(j.api, plan, chartemit.EmitEnv{
		TypeID:      j.fullName,
		UpdateEvery: j.updateEvery,
		Plugin:      j.pluginName,
		Module:      j.moduleName,
		JobName:     j.name,
		JobLabels:   j.labels,
		MSSinceLast: sinceLastRun,
	}); err != nil {
		j.Warningf("apply plan failed: %v", err)
		return false
	}
	return true
}

func (j *JobV2) penalty() int {
	v := j.retries / penaltyStep * penaltyStep * j.updateEvery / 2
	if v > maxPenalty {
		return maxPenalty
	}
	return v
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
