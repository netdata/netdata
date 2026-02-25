// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

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
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/tickstate"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type JobV2Config struct {
	PluginName      string
	Name            string
	ModuleName      string
	FullName        string
	Module          collectorapi.CollectorV2
	Labels          map[string]string
	Out             io.Writer
	UpdateEvery     int
	AutoDetectEvery int
	IsStock         bool
	Vnode           vnodes.VirtualNode
	FunctionOnly    bool
	RuntimeService  runtimecomp.Service
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
		autoDetectTries: infTries,
		isStock:         cfg.IsStock,
		functionOnly:    cfg.FunctionOnly,
		module:          cfg.Module,
		labels:          cloneLabels(cfg.Labels),
		out:             cfg.Out,
		stopCtrl:        newStopController(),
		tick:            make(chan int),
		updVnode:        make(chan *vnodes.VirtualNode, 1),
		buf:             &buf,
		api:             netdataapi.New(&buf),
		vnode:           cfg.Vnode,
		runtimeService:  cfg.RuntimeService,
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
	autoDetectTries int
	isStock         bool
	functionOnly    bool
	labels          map[string]string

	*logger.Logger

	module collectorapi.CollectorV2

	running atomic.Bool

	initialized bool
	panicked    atomic.Bool

	store  metrix.CollectorStore
	cycle  metrix.CycleController
	engine *chartengine.Engine

	prevRun time.Time
	retries atomic.Int64

	vnodeMu  sync.RWMutex
	vnode    vnodes.VirtualNode
	updVnode chan *vnodes.VirtualNode

	ctxMu     sync.RWMutex
	runCtx    context.Context
	cancelRun context.CancelFunc

	tick chan int
	out  io.Writer
	buf  *bytes.Buffer
	api  *netdataapi.API

	stopCtrl stopController

	runtimeService             runtimecomp.Service
	runtimeComponentName       string
	runtimeComponentRegistered bool

	skipTracker tickstate.SkipTracker
}

func (j *JobV2) FullName() string                 { return j.fullName }
func (j *JobV2) ModuleName() string               { return j.moduleName }
func (j *JobV2) Name() string                     { return j.name }
func (j *JobV2) Panicked() bool                   { return j.panicked.Load() }
func (j *JobV2) IsRunning() bool                  { return j.running.Load() }
func (j *JobV2) Module() collectorapi.CollectorV2 { return j.module }
func (j *JobV2) Collector() any                   { return j.module }
func (j *JobV2) AutoDetectionEvery() int {
	return j.autoDetectEvery
}
func (j *JobV2) RetryAutoDetection() bool {
	return retryAutoDetection(j.autoDetectEvery, j.autoDetectTries)
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
	select {
	case <-j.updVnode:
	default:
	}
	j.updVnode <- vnode
}
func (j *JobV2) Cleanup() {
	j.unregisterRuntimeComponent()
	if j.module != nil {
		j.module.Cleanup(context.Background())
	}
}

func (j *JobV2) AutoDetection() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic %v", r)
			j.panicked.Store(true)
			j.disableAutoDetection()
			j.Errorf("PANIC %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
		if err != nil {
			j.Cleanup()
		}
	}()
	if j.isStock {
		j.Mute()
	}

	if err = j.init(); err != nil {
		j.Errorf("init failed: %v", err)
		j.Unmute()
		j.disableAutoDetection()
		return err
	}
	if err = j.check(); err != nil {
		j.Errorf("check failed: %v", err)
		j.Unmute()
		return err
	}
	j.Unmute()
	j.Info("check success")
	if err = j.postCheck(); err != nil {
		j.Errorf("postCheck failed: %v", err)
		j.disableAutoDetection()
		return err
	}
	return nil
}

func (j *JobV2) Start() {
	j.stopCtrl.markStarted()
	j.running.Store(true)
	runCtx, cancel := context.WithCancel(context.Background())
	j.setRunContext(runCtx, cancel)
	if j.functionOnly {
		j.Info("started in function-only mode")
	} else {
		j.Infof("started (v2), data collection interval %ds", j.updateEvery)
	}
	defer func() {
		cancel()
		j.setRunContext(nil, nil)
		j.stopCtrl.markStopped()
		j.Info("stopped")
	}()

LOOP:
	for {
		select {
		case <-j.stopCtrl.stopCh:
			break LOOP
		case t := <-j.tick:
			if !j.functionOnly && j.shouldCollect(t) {
				markRunStartWithResumeLog(&j.skipTracker, j.Logger)
				j.runOnce()
				j.skipTracker.MarkRunStop(time.Now())
			}
		}
	}
	// Mark not-running before cleanup so external function dispatch can reject requests
	// while module resources are being torn down.
	j.running.Store(false)
	j.Cleanup()
}

func (j *JobV2) Stop() {
	j.cancelRunContext()
	j.stopCtrl.stopAndWait()
}

func (j *JobV2) Tick(clock int) {
	enqueueTickWithSkipLog(j.tick, clock, j.functionOnly, j.updateEvery, int(j.retries.Load()), &j.skipTracker, j.Logger)
}

func (j *JobV2) shouldCollect(clock int) bool {
	return shouldCollectWithPenalty(clock, j.updateEvery, int(j.retries.Load()))
}

func (j *JobV2) init() error {
	if j.initialized {
		return nil
	}
	if err := j.module.Init(j.moduleContext()); err != nil {
		return err
	}
	j.initialized = true
	return nil
}

func (j *JobV2) check() error {
	if err := j.module.Check(j.moduleContext()); err != nil {
		consumeAutoDetectTry(&j.autoDetectTries)
		return err
	}
	return nil
}

func (j *JobV2) postCheck() error {
	if j.functionOnly {
		// Match v1 semantics: function-only jobs validate connectivity only.
		return nil
	}

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
	if v, ok := j.module.(collectorapi.CollectorV2EnginePolicy); ok {
		policy := v.EnginePolicy()
		// Chartengine autogen type.id budget must use the actual emitted type.id.
		// JobV2 always emits with fullName as TypeID.
		policy.Autogen.TypeID = j.fullName
		opts = append(opts, chartengine.WithEnginePolicy(policy))
	}

	engine, err := chartengine.New(opts...)
	if err != nil {
		return err
	}
	if err := engine.LoadYAML([]byte(j.module.ChartTemplateYAML()), 1); err != nil {
		return err
	}

	j.store = store
	j.cycle = managed.CycleController()
	j.engine = engine
	if err := j.registerRuntimeComponent(); err != nil {
		j.Warningf("runtime metrics registration failed: %v", err)
	}
	return nil
}

func (j *JobV2) runOnce() {
	j.applyPendingVnodeUpdate()

	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.prevRun = curTime

	ok := j.collectAndEmit(sinceLastRun)
	if ok {
		j.retries.Store(0)
	} else {
		j.retries.Add(1)
	}

	// Never flush buffered output from failed or panicked cycles:
	// a panic can leave partial protocol lines in the buffer.
	if ok && !j.panicked.Load() {
		_, _ = io.Copy(j.out, j.buf)
	}
	j.buf.Reset()
}

func (j *JobV2) applyPendingVnodeUpdate() {
	select {
	case vnode := <-j.updVnode:
		if vnode == nil {
			return
		}
		if j.module != nil && j.module.VirtualNode() != nil {
			// Match v1 ownership model: do not override module-owned vnode state.
			return
		}

		next := vnode.Copy()

		j.vnodeMu.Lock()
		j.vnode = *next
		j.vnodeMu.Unlock()
	default:
	}
}

func (j *JobV2) collectAndEmit(sinceLastRun int) bool {
	j.panicked.Store(false)
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
			j.panicked.Store(true)
			j.Errorf("PANIC: %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	j.cycle.BeginCycle()
	cycleOpen = true
	if err := j.module.Collect(j.moduleContext()); err != nil {
		j.cycle.AbortCycle()
		cycleOpen = false
		j.Warningf("collect failed: %v", err)
		return false
	}
	j.cycle.CommitCycleSuccess()
	cycleOpen = false

	plan, err := j.engine.BuildPlan(j.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
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
	return penaltyFromRetries(int(j.retries.Load()), j.updateEvery)
}

func (j *JobV2) disableAutoDetection() {
	disableAutoDetection(&j.autoDetectEvery)
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

func (j *JobV2) moduleContext() context.Context {
	j.ctxMu.RLock()
	ctx := j.runCtx
	j.ctxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (j *JobV2) cancelRunContext() {
	j.ctxMu.RLock()
	cancel := j.cancelRun
	j.ctxMu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (j *JobV2) setRunContext(ctx context.Context, cancel context.CancelFunc) {
	j.ctxMu.Lock()
	j.runCtx = ctx
	j.cancelRun = cancel
	j.ctxMu.Unlock()
}
