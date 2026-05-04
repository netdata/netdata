// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
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
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type JobV2Config struct {
	PluginName      string
	Name            string
	ModuleName      string
	FullName        string
	Source          string
	Module          collectorapi.CollectorV2
	Labels          map[string]string
	Out             io.Writer
	UpdateEvery     int
	AutoDetectEvery int
	IsStock         bool
	Vnode           vnodes.VirtualNode
	VnodeRegistry   *vnoderegistry.Registry
	FunctionOnly    bool
	RuntimeService  runtimecomp.Service
}

func NewJobV2(cfg JobV2Config) *JobV2 {
	var buf bytes.Buffer
	if cfg.UpdateEvery <= 0 {
		cfg.UpdateEvery = 1
	}
	registry := cfg.VnodeRegistry
	if registry == nil {
		registry = vnoderegistry.New()
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
		vnodeRegistry:   registry,
		runtimeService:  cfg.RuntimeService,
	}
	if j.out == nil {
		j.out = io.Discard
	}

	log := logger.New().With(jobLoggerAttrs(j.ModuleName(), j.Name(), cfg.Source)...)
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

	store metrix.CollectorStore
	cycle metrix.CycleController

	scopeStates           map[string]*jobV2ScopeState
	chartTemplateYAML     []byte
	chartTemplateRevision uint64
	engineOptions         []chartengine.Option
	runtimeStore          metrix.RuntimeStore
	runtimeAggregator     *chartengine.RuntimeAggregator

	prevRun time.Time
	retries atomic.Int64

	vnodeMu  sync.RWMutex
	vnode    vnodes.VirtualNode
	updVnode chan *vnodes.VirtualNode

	vnodeRegistry *vnoderegistry.Registry

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

type jobV2PreparedEmission struct {
	scopes       []jobV2PreparedScopeEmission
	scopeFailure bool
}

type jobV2PreparedScopeEmission struct {
	scope    *jobV2ScopeState
	attempt  chartengine.PlanAttempt
	plan     chartengine.Plan
	decision jobV2EmissionDecision
	output   []byte
	live     bool
}

type jobV2ScopeState struct {
	scopeKey string
	scope    metrix.HostScope
	engine   *chartengine.Engine
	host     jobV2HostState
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
	j.buf.Reset()
	snapshots := j.captureScopeCleanupSnapshots()
	j.unregisterRuntimeComponent()
	if j.module != nil {
		j.module.Cleanup(context.Background())
	}
	if !collectorapi.ShouldObsoleteCharts() {
		j.releaseAllScopeRegistryOwners()
		j.clearAllScopeStateAfterCleanup()
		return
	}

	for _, snapshot := range snapshots {
		if snapshot.staleVnodeSuppressed || len(snapshot.charts) == 0 {
			continue
		}

		env := chartemit.EmitEnv{
			TypeID:      j.fullName,
			UpdateEvery: j.updateEvery,
			Plugin:      j.pluginName,
			Module:      j.moduleName,
			JobName:     j.name,
			JobLabels:   j.labels,
		}
		if snapshot.host.isVnode() {
			env.HostScope = &chartemit.HostScope{GUID: snapshot.host.guid}
		}
		j.buf.Reset()
		if err := chartemit.ApplyPlan(j.api, buildJobV2CleanupPlan(snapshot.charts), env); err != nil {
			j.Warningf("cleanup apply plan failed for host scope %q: %v", snapshot.scopeKey, err)
			j.buf.Reset()
			continue
		}
		_, _ = io.Copy(j.out, j.buf)
		j.buf.Reset()
	}
	j.releaseAllScopeRegistryOwners()
	j.clearAllScopeStateAfterCleanup()
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
		chartengine.WithEmitTypeIDBudgetPrefix(j.fullName),
	}
	if v, ok := j.module.(collectorapi.CollectorV2EnginePolicy); ok {
		opts = append(opts, chartengine.WithEnginePolicy(v.EnginePolicy()))
	}

	templateYAML := []byte(j.module.ChartTemplateYAML())
	if err := validateJobV2ChartTemplate(templateYAML, opts); err != nil {
		return err
	}

	j.store = store
	j.cycle = managed.CycleController()
	j.scopeStates = make(map[string]*jobV2ScopeState)
	j.chartTemplateYAML = templateYAML
	j.chartTemplateRevision = 1
	j.engineOptions = opts
	j.runtimeStore = metrix.NewRuntimeStore()
	j.runtimeAggregator = chartengine.NewRuntimeAggregator(j.runtimeStore)
	if err := j.registerRuntimeComponent(); err != nil {
		j.Warningf("runtime metrics registration failed: %v", err)
	}
	return nil
}

func validateJobV2ChartTemplate(templateYAML []byte, opts []chartengine.Option) error {
	engineOpts := append([]chartengine.Option{}, opts...)
	engineOpts = append(engineOpts, chartengine.WithRuntimeStore(nil))
	engine, err := chartengine.New(engineOpts...)
	if err != nil {
		return err
	}
	return engine.LoadYAML(templateYAML, 1)
}

func (j *JobV2) runOnce() {
	defer j.ResetAllOnce()
	defer j.flushRuntimeAggregator()

	j.applyPendingVnodeUpdate()

	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.prevRun = curTime

	prepared, ok := j.collectAndEmit(sinceLastRun)
	if ok && !j.panicked.Load() {
		if err := j.finishPreparedEmission(prepared); err != nil {
			j.Warningf("finalize emission failed: %v", err)
			ok = false
		}
	}
	if ok {
		j.retries.Store(0)
	} else {
		j.retries.Add(1)
	}
	j.buf.Reset()
}

func (j *JobV2) flushRuntimeAggregator() {
	if j != nil && j.runtimeAggregator != nil {
		j.runtimeAggregator.Flush()
	}
}

func (j *JobV2) applyPendingVnodeUpdate() {
	select {
	case vnode := <-j.updVnode:
		if vnode == nil {
			return
		}
		if j.module != nil && j.module.VirtualNode() != nil {
			// Match v1 ownership model: do not override module-owned vnode state.
			j.Debugf("ignoring vnode update for module-owned vnode")
			return
		}

		next := vnode.Copy()

		j.vnodeMu.Lock()
		j.vnode = *next
		j.vnodeMu.Unlock()
		// Registry owner release is intentionally tied to the next successful
		// emission or cleanup, so obsolete emission can still select the old host.
		if state := j.scopeStates[defaultHostScopeKey]; state != nil {
			state.host.invalidateDefine()
		}
	default:
	}
}

func (j *JobV2) collectAndEmit(sinceLastRun int) (prepared jobV2PreparedEmission, ok bool) {
	j.panicked.Store(false)
	cycleOpen := false

	defer func() {
		if r := recover(); r != nil {
			j.rollbackPreparedEmission(prepared)
			j.buf.Reset()
			if j.runtimeAggregator != nil {
				j.runtimeAggregator.Reset()
			}
			if cycleOpen {
				// Recover path must close staged frame to keep subsequent cycles valid.
				func() {
					defer func() { _ = recover() }()
					j.cycle.AbortCycle()
				}()
			}
			j.abortPreparedEmission(prepared)
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
		return jobV2PreparedEmission{}, false
	}
	if err := j.cycle.CommitCycleSuccess(); err != nil {
		cycleOpen = false
		j.Warningf("commit cycle failed: %v", err)
		return jobV2PreparedEmission{}, false
	}
	cycleOpen = false

	liveSet := j.liveScopeSet()
	workSet := j.scopeWorkSet(liveSet)
	for _, scopeKey := range sortedScopeKeys(workSet) {
		scope := workSet[scopeKey]
		_, live := liveSet[scopeKey]
		if !live {
			if state := j.scopeStates[scopeKey]; state != nil {
				scope = state.scope
			}
		}
		scopePrepared, scopeOK := j.prepareScopeEmission(scope, live, sinceLastRun)
		if !scopeOK {
			prepared.scopeFailure = true
			continue
		}
		prepared.scopes = append(prepared.scopes, scopePrepared)
	}
	j.Debugf("v2 scope count: %d", len(j.scopeStates))
	if len(prepared.scopes) == 0 && prepared.scopeFailure {
		return prepared, false
	}
	return prepared, true
}

func (j *JobV2) finishPreparedEmission(prepared jobV2PreparedEmission) error {
	successes := 0
	failures := 0
	var finalErr error
	for _, scope := range prepared.scopes {
		if err := scope.attempt.Commit(); err != nil {
			j.rollbackVnodeRegistryEmission(scope.decision)
			failures++
			finalErr = errors.Join(finalErr, err)
			j.Warningf("finalize emission for host scope %q failed: %v", scope.scope.scopeKey, err)
			continue
		}
		if len(scope.output) > 0 {
			_, _ = j.out.Write(scope.output)
		}
		j.commitScopeEmission(scope)
		successes++
	}
	if prepared.scopeFailure {
		failures++
	}
	if successes == 0 && failures > 0 {
		if finalErr != nil {
			return finalErr
		}
		return fmt.Errorf("all host scope emissions failed")
	}
	return nil
}

func (j *JobV2) prepareScopeEmission(scope metrix.HostScope, live bool, sinceLastRun int) (prepared jobV2PreparedScopeEmission, ok bool) {
	var attempt chartengine.PlanAttempt
	var decision jobV2EmissionDecision
	defer func() {
		if r := recover(); r != nil {
			j.rollbackVnodeRegistryEmission(decision)
			attempt.Abort()
			j.buf.Reset()
			panic(r)
		}
		if !ok {
			j.rollbackVnodeRegistryEmission(decision)
			attempt.Abort()
			j.buf.Reset()
		}
	}()

	state, err := j.ensureScopeState(scope)
	if err != nil {
		j.Warningf("prepare host scope %q failed: %v", scope.ScopeKey, err)
		return jobV2PreparedScopeEmission{}, false
	}

	if state.scopeKey == defaultHostScopeKey {
		vnode := j.currentVnode()
		decision, err = state.host.prepareEmission(vnode)
		if err == nil && decision.needEngineReload {
			state.engine.ResetMaterialized()
		}
		if err != nil {
			j.Warningf("prepare default host scope failed: %v", err)
			return jobV2PreparedScopeEmission{}, false
		}
	} else {
		decision, err = state.host.prepareScopedEmission(state.scope)
		if err == nil && decision.needEngineReload {
			state.engine.ResetMaterialized()
		}
		if err != nil {
			j.Warningf("prepare host scope %q failed: %v", state.scopeKey, err)
			return jobV2PreparedScopeEmission{}, false
		}
	}

	attempt, err = state.engine.PreparePlan(j.store.Read(metrix.ReadRaw(), metrix.ReadFlatten(), metrix.ReadHostScope(state.scopeKey)))
	if err != nil {
		j.Warningf("build plan for host scope %q failed: %v", state.scopeKey, err)
		return jobV2PreparedScopeEmission{}, false
	}
	plan := attempt.Plan()
	if err := j.prepareScopeVnodeRegistryEmission(state, &decision, plan); err != nil {
		j.Warningf("prepare vnode registry for host scope %q failed: %v", state.scopeKey, err)
		return jobV2PreparedScopeEmission{}, false
	}

	j.buf.Reset()
	env := j.emitEnv(sinceLastRun, decision)
	if err := chartemit.ApplyPlan(j.api, plan, env); err != nil {
		j.Warningf("apply plan for host scope %q failed: %v", state.scopeKey, err)
		return jobV2PreparedScopeEmission{}, false
	}
	output := append([]byte(nil), j.buf.Bytes()...)
	j.buf.Reset()

	prepared = jobV2PreparedScopeEmission{
		scope:    state,
		attempt:  attempt,
		plan:     plan,
		decision: decision,
		output:   output,
		live:     live,
	}
	return prepared, true
}

func (j *JobV2) commitScopeEmission(prepared jobV2PreparedScopeEmission) {
	if prepared.scope == nil {
		return
	}
	state := prepared.scope
	if state.scopeKey == defaultHostScopeKey || prepared.decision.registryOwner != "" {
		keep := make(map[vnoderegistry.Owner]struct{}, 1)
		if prepared.decision.registryOwner != "" {
			keep[prepared.decision.registryOwner] = struct{}{}
		}
		state.host.releaseSupersededRegistryOwnersExcept(
			j.vnodeRegistry,
			keep,
			j.vnodeRegistryOwnerNamespacePrefix(state.scopeKey),
		)
	}
	state.host.commitSuccessfulEmission(prepared.plan, prepared.decision)
	if !prepared.live && len(state.host.cleanupCharts) == 0 {
		state.host.releaseRegistryOwners(j.vnodeRegistry)
		delete(j.scopeStates, state.scopeKey)
	}
}

func (j *JobV2) rollbackPreparedEmission(prepared jobV2PreparedEmission) {
	for _, scope := range prepared.scopes {
		j.rollbackVnodeRegistryEmission(scope.decision)
	}
}

func (j *JobV2) abortPreparedEmission(prepared jobV2PreparedEmission) {
	for _, scope := range prepared.scopes {
		scope.attempt.Abort()
	}
}

func (j *JobV2) emitEnv(sinceLastRun int, decision jobV2EmissionDecision) chartemit.EmitEnv {
	env := chartemit.EmitEnv{
		TypeID:      j.fullName,
		UpdateEvery: j.updateEvery,
		Plugin:      j.pluginName,
		Module:      j.moduleName,
		JobName:     j.name,
		JobLabels:   j.labels,
		MSSinceLast: sinceLastRun,
	}
	env.HostScope = decision.hostScope
	return env
}

func (j *JobV2) currentVnode() vnodes.VirtualNode {
	if j.module != nil {
		if vnode := j.module.VirtualNode(); vnode != nil {
			return *vnode.Copy()
		}
	}
	j.vnodeMu.RLock()
	defer j.vnodeMu.RUnlock()
	return *j.vnode.Copy()
}

func (j *JobV2) prepareScopeVnodeRegistryEmission(state *jobV2ScopeState, decision *jobV2EmissionDecision, plan chartengine.Plan) error {
	if decision == nil || !decision.targetHost.isVnode() || len(plan.Actions) == 0 {
		return nil
	}
	if state == nil {
		return fmt.Errorf("nil host scope state")
	}
	if state.scopeKey == defaultHostScopeKey {
		vnode := j.currentVnode()
		return j.prepareVnodeRegistryEmission(decision, j.vnodeRegistryOwner(decision.targetHost), netdataapi.HostInfo{
			GUID:     vnode.GUID,
			Hostname: vnode.Hostname,
			Labels:   vnode.Labels,
		})
	}
	return j.prepareVnodeRegistryEmission(decision, j.vnodeRegistryScopedOwner(state.scopeKey, state.scope.GUID), metrixHostScopeInfo(state.scope))
}

func (j *JobV2) prepareVnodeRegistryEmission(decision *jobV2EmissionDecision, owner vnoderegistry.Owner, info netdataapi.HostInfo) error {
	registryInfo := netdataapi.HostInfo{
		GUID:     info.GUID,
		Hostname: info.Hostname,
		Labels:   maps.Clone(info.Labels),
	}
	result, err := j.vnodeRegistry.Register(owner, registryInfo)
	if err != nil {
		return err
	}
	if result.MetadataUpdated && result.UpdateFirstSeen {
		j.Warningf(
			"vnode registry metadata updated for guid %q: hostname %q replaced by %q",
			result.Info.GUID,
			result.Previous.Hostname,
			result.Info.Hostname,
		)
	}

	scope := &chartemit.HostScope{GUID: decision.targetHost.guid}
	if result.NeedDefine {
		scope.Define = &result.Info
	}
	decision.hostScope = scope
	decision.defineInfo = result.Info
	decision.registryOwner = owner
	decision.registryRegistration = result
	return nil
}

func (j *JobV2) rollbackVnodeRegistryEmission(decision jobV2EmissionDecision) {
	if decision.registryOwner != "" {
		j.vnodeRegistry.Rollback(decision.registryOwner, decision.registryRegistration)
	}
}

const vnodeRegistryOwnerSeparator = "\xff"

func (j *JobV2) vnodeRegistryOwnerPrefix() string {
	// Keep the separator outside valid metrix scope keys and GUIDs so owner
	// strings remain unambiguous without allocating a structured key.
	return j.fullName + vnodeRegistryOwnerSeparator
}

func (j *JobV2) vnodeRegistryJobOwnerPrefix() string {
	// Keep job-level vnode owners separate from future per-scope owners.
	return j.vnodeRegistryOwnerPrefix() + "job" + vnodeRegistryOwnerSeparator
}

func (j *JobV2) vnodeRegistryScopedOwnerPrefix(scopeKey string) string {
	return j.vnodeRegistryOwnerPrefix() + "scope" + vnodeRegistryOwnerSeparator + scopeKey + vnodeRegistryOwnerSeparator
}

func (j *JobV2) vnodeRegistryOwnerNamespacePrefix(scopeKey string) string {
	if scopeKey == defaultHostScopeKey {
		return j.vnodeRegistryJobOwnerPrefix()
	}
	return j.vnodeRegistryScopedOwnerPrefix(scopeKey)
}

func (j *JobV2) vnodeRegistryOwner(target jobV2HostRef) vnoderegistry.Owner {
	return vnoderegistry.Owner(j.vnodeRegistryJobOwnerPrefix() + target.guid)
}

func (j *JobV2) vnodeRegistryScopedOwner(scopeKey, guid string) vnoderegistry.Owner {
	return vnoderegistry.Owner(j.vnodeRegistryScopedOwnerPrefix(scopeKey) + guid)
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
	maps.Copy(out, in)
	return out
}

func (j *JobV2) moduleContext() context.Context {
	j.ctxMu.RLock()
	ctx := j.runCtx
	j.ctxMu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if j.runtimeService != nil {
		return runtimecomp.ContextWithService(ctx, j.runtimeService)
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
