// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/tickstate"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func newCollectStatusChart(pluginName string) *collectorapi.Chart {
	chart := &collectorapi.Chart{
		Title:    "Data Collection Status",
		Units:    "status",
		Fam:      pluginName,
		Ctx:      "netdata.plugin_data_collection_status",
		Priority: 144000,
		Dims: collectorapi.Dims{
			{ID: "success"},
			{ID: "failed"},
		},
	}
	chart.SetCachedType("netdata")
	return chart
}

func newCollectDurationChart(pluginName string) *collectorapi.Chart {
	chart := &collectorapi.Chart{
		Title:    "Data Collection Duration",
		Units:    "ms",
		Fam:      pluginName,
		Ctx:      "netdata.plugin_data_collection_duration",
		Priority: 145000,
		Dims: collectorapi.Dims{
			{ID: "duration"},
		},
	}
	chart.SetCachedType("netdata")
	return chart
}

type JobConfig struct {
	PluginName              string
	Name                    string
	ModuleName              string
	FullName                string
	Source                  string
	Module                  collectorapi.CollectorV1
	Labels                  map[string]string
	Out                     io.Writer
	CleanupOut              io.Writer // terminal cleanup sink; defaults to Out
	UpdateEvery             int
	AutoDetectEvery         int
	Priority                int
	IsStock                 bool
	Vnode                   vnodes.VirtualNode
	VnodeName               string
	VnodeRevision           uint64
	VnodeMetadataRevision   uint64
	Publication             *hostoutput.Publisher
	VnodeLookup             VnodeLookup
	FunctionOnly            bool
	LifecycleErrorSanitizer func(error) error
}

func NewJob(cfg JobConfig) *Job {
	var buf bytes.Buffer

	if cfg.UpdateEvery == 0 {
		cfg.UpdateEvery = 1
	}
	if cfg.CleanupOut == nil {
		cfg.CleanupOut = cfg.Out
	}

	if cfg.Publication == nil {
		cfg.Publication = hostoutput.New()
	}
	j := &Job{
		publication:     cfg.Publication,
		hostCharts:      make(jobV1ChartInventory),
		selfCharts:      make(jobV1ChartInventory),
		autoDetectEvery: cfg.AutoDetectEvery,
		autoDetectTries: infTries,

		pluginName:              cfg.PluginName,
		name:                    cfg.Name,
		moduleName:              cfg.ModuleName,
		fullName:                cfg.FullName,
		updateEvery:             cfg.UpdateEvery,
		priority:                cfg.Priority,
		isStock:                 cfg.IsStock,
		functionOnly:            cfg.FunctionOnly,
		module:                  cfg.Module,
		labels:                  cfg.Labels,
		out:                     cfg.Out,
		cleanupOut:              cfg.CleanupOut,
		collectStatusChart:      newCollectStatusChart(cfg.PluginName),
		collectDurationChart:    newCollectDurationChart(cfg.PluginName),
		stopCtrl:                newStopController(),
		tick:                    make(chan int),
		buf:                     &buf,
		api:                     netdataapi.New(&buf),
		vnode:                   cfg.Vnode,
		vnodeName:               cfg.VnodeName,
		vnodeRevision:           cfg.VnodeRevision,
		vnodeMetadataRevision:   cfg.VnodeMetadataRevision,
		vnodeLookup:             cfg.VnodeLookup,
		lifecycleErrorSanitizer: cfg.LifecycleErrorSanitizer,
	}

	j.collectStatusChart.ID = fmt.Sprintf("%s_%s_data_collection_status", cleanPluginName(j.pluginName), j.FullName())
	j.collectDurationChart.ID = fmt.Sprintf(
		"%s_%s_data_collection_duration",
		cleanPluginName(j.pluginName),
		j.FullName(),
	)
	log := logger.New().With(jobLoggerAttrs(j.ModuleName(), j.Name(), cfg.Source)...)

	j.Logger = log
	if j.module != nil {
		moduleLog := log
		if sanitize := lifecycleLogMessageSanitizer(cfg.LifecycleErrorSanitizer); sanitize != nil {
			moduleLog = moduleLog.WithMessageSanitizer(sanitize)
		}
		j.module.GetBase().Logger = moduleLog
	}

	return j
}

// Job represents a job. It's a module wrapper.
type Job struct {
	pluginName string
	name       string
	moduleName string
	fullName   string

	updateEvery     int
	autoDetectEvery int
	autoDetectTries int
	priority        int
	labels          map[string]string

	*logger.Logger

	isStock      bool
	functionOnly bool

	module                  collectorapi.CollectorV1
	lifecycleErrorSanitizer func(error) error

	// running tracks whether the managed job loop is active.
	running atomic.Bool

	initialized bool
	panicked    atomic.Bool

	collectStatusChart   *collectorapi.Chart
	collectDurationChart *collectorapi.Chart
	charts               *collectorapi.Charts
	tick                 chan int
	out                  io.Writer
	cleanupOut           io.Writer
	buf                  *bytes.Buffer
	api                  *netdataapi.API

	publication    *hostoutput.Publisher
	hostOwner      *hostoutput.Owner
	hostGUID       string
	hostDefinition *hostoutput.Definition
	hostCharts     jobV1ChartInventory
	selfCharts     jobV1ChartInventory
	emission       jobV1Emission
	// vnodeMu covers current vnode state while collection refreshes it.
	vnodeMu               sync.RWMutex
	vnode                 vnodes.VirtualNode
	vnodeName             string
	vnodeRevision         uint64
	vnodeMetadataRevision uint64
	vnodeLookup           VnodeLookup

	retries atomic.Int64
	prevRun time.Time

	stopCtrl stopController

	// moduleCleanup guards explicit accepted/rejected lifecycle cleanup to
	// exactly once.
	moduleCleanup sync.Once

	skipTracker tickstate.SkipTracker
}

type collectedMetrics struct {
	intMetrics   map[string]int64
	floatMetrics map[string]float64 // not used, only v2 collectors will have float metrics
}

func (cm *collectedMetrics) getValue(id string) (float64, bool) {
	if v, ok := cm.floatMetrics[id]; ok {
		return v, true
	}
	v, ok := cm.intMetrics[id]
	return float64(v), ok
}

// NetdataChartIDMaxLength is the chart ID max length. See RRD_ID_LENGTH_MAX in the netdata source code.
const NetdataChartIDMaxLength = 1200

// FullName returns job full name.
func (j *Job) FullName() string {
	return j.fullName
}

// ModuleName returns job module name.
func (j *Job) ModuleName() string {
	return j.moduleName
}

// Name returns job name.
func (j *Job) Name() string {
	return j.name
}

// AutoDetectionEvery returns the autodetection retry cadence.
func (j *Job) AutoDetectionEvery() int {
	return j.autoDetectEvery
}

// RetryAutoDetection returns whether it is needed to retry autodetection.
func (j *Job) RetryAutoDetection() bool {
	return retryAutoDetection(j.autoDetectEvery, j.autoDetectTries)
}

// AutoDetectionManaged leaves failure cleanup with the Job Manager factory.
func (j *Job) AutoDetectionManaged(ctx context.Context) (err error) {
	return j.autoDetection(ctx)
}

func (j *Job) autoDetection(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = sanitizeLifecycleError(j.lifecycleErrorSanitizer, fmt.Errorf("panic %v", r))
			j.panicked.Store(true)
			j.disableAutoDetection()

			j.Errorf("PANIC %v", err)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	if j.isStock {
		j.Mute()
	}

	if rawErr := j.init(ctx); rawErr != nil {
		if !isRetryableError(rawErr) {
			j.disableAutoDetection()
		}
		err = sanitizeLifecycleError(j.lifecycleErrorSanitizer, rawErr)
		j.Errorf("init failed: %v", err)
		j.Unmute()
		return err
	}

	if rawErr := j.check(ctx); rawErr != nil {
		err = sanitizeLifecycleError(j.lifecycleErrorSanitizer, rawErr)
		j.Errorf("check failed: %v", err)
		j.Unmute()
		return err
	}

	j.Unmute()
	j.Info("check success")

	if rawErr := j.postCheck(); rawErr != nil {
		err = sanitizeLifecycleError(j.lifecycleErrorSanitizer, rawErr)
		j.Errorf("postCheck failed: %v", err)
		j.disableAutoDetection()
		return err
	}

	return nil
}

func (j *Job) refreshVnodeSnapshot() {
	if j.vnodeName == "" || j.vnodeLookup == nil {
		return
	}
	snapshot, ok := j.vnodeLookup(j.vnodeName)
	if !ok {
		return
	}
	j.applyVnodeSnapshot(snapshot)
}

func (j *Job) applyVnodeSnapshot(snapshot VnodeSnapshot) {
	if snapshot.Vnode == nil {
		return
	}
	if snapshot.Revision != 0 {
		j.vnodeMu.Lock()
		stale := snapshot.Revision <= j.vnodeRevision
		j.vnodeMu.Unlock()
		if stale {
			return
		}
	}
	next := snapshot.Vnode.Copy()

	j.vnodeMu.Lock()
	defer j.vnodeMu.Unlock()

	if snapshot.Revision != 0 && snapshot.Revision <= j.vnodeRevision {
		return
	}

	j.vnode = *next
	if snapshot.Revision != 0 {
		j.vnodeRevision = snapshot.Revision
	}
	if snapshot.MetadataRevision != 0 {
		j.vnodeMetadataRevision = snapshot.MetadataRevision
	}
}

// Tick Tick.
func (j *Job) Tick(clock int) {
	enqueueTickWithSkipLog(
		j.tick,
		clock,
		j.functionOnly,
		j.updateEvery,
		int(j.retries.Load()),
		&j.skipTracker,
		j.Logger,
	)
}

// IsRunning returns true if the job's main loop is currently running.
// This is safe to call from any goroutine.
func (j *Job) IsRunning() bool {
	return j.running.Load()
}

// Collector returns the underlying collector instance bound to this job.
func (j *Job) Collector() any {
	return j.module
}

// StartManaged starts the collector loop while leaving Cleanup ownership with
// the caller. It acknowledges readiness only after the loop has published its
// running state.
func (j *Job) StartManaged(ready chan<- struct{}) {
	j.run(ready)
}

func (j *Job) run(ready chan<- struct{}) {
	j.stopCtrl.markStarted()
	j.running.Store(true)
	if ready != nil {
		close(ready)
	}
	if j.functionOnly {
		j.Info("started in function-only mode")
	} else {
		j.Infof("started, data collection interval %ds", j.updateEvery)
	}
	defer func() {
		j.running.Store(false)
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
}

// Stop stops job main loop. It blocks until the job is stopped.
func (j *Job) Stop() {
	j.stopCtrl.stopAndWait()
}

func (j *Job) shouldCollect(clock int) bool {
	return shouldCollectWithPenalty(clock, j.updateEvery, int(j.retries.Load()))
}

func (j *Job) disableAutoDetection() {
	disableAutoDetection(&j.autoDetectEvery)
}

func (j *Job) cleanupModule() {
	j.moduleCleanup.Do(func() { j.module.Cleanup(context.TODO()) })
}

func (j *Job) Cleanup() {
	defer j.clearOutputState()
	j.cleanupModule()
	j.buf.Reset()
	if !collectorapi.ShouldObsoleteCharts() {
		return
	}
	if len(j.hostCharts) > 0 {
		j.api.HOST(j.hostGUID)
		j.hostCharts.cleanup(j.api)
	}
	hostBytes := j.buf.Len()
	if len(j.selfCharts) > 0 {
		j.api.HOST("")
		j.selfCharts.cleanup(j.api)
	}
	if j.buf.Len() > 0 {
		if _, err := commitHostOutput(j.cleanupOut, hostoutput.Request{
			Owner:      j.hostOwner,
			Definition: j.hostDefinition,
			Payload:    j.buf.Bytes()[:hostBytes],
			Tail:       j.buf.Bytes()[hostBytes:],
			Cleanup:    true,
		}, nil); err != nil {
			j.Errorf("cleanup output failed: %v", err)
		}
	}
	j.buf.Reset()
}

// CleanupRejected releases a constructed job without emitting cleanup output.
func (j *Job) CleanupRejected() {
	defer j.clearOutputState()
	j.cleanupModule()
	j.buf.Reset()
}

func (j *Job) init(ctx context.Context) error {
	if j.initialized {
		return nil
	}

	if err := j.module.Init(ctx); err != nil {
		return err
	}

	j.initialized = true

	return nil
}

func (j *Job) check(ctx context.Context) error {
	if err := j.module.Check(ctx); err != nil {
		consumeAutoDetectTry(&j.autoDetectTries)
		return err
	}
	return nil
}

func (j *Job) postCheck() error {
	j.charts = j.module.Charts()
	if j.charts == nil && !j.functionOnly {
		j.Error("nil charts")
		return errors.New("nil charts")
	}
	if j.charts != nil {
		if err := collectorapi.CheckCharts(*j.charts...); err != nil {
			return err
		}
	}
	return nil
}

func (j *Job) runOnce() {
	defer j.ResetAllOnce()
	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.refreshVnodeSnapshot()
	metrics := j.collect()
	if j.panicked.Load() {
		return
	}
	if j.vnodeName == "" && j.vnode.GUID == "" {
		if v := j.module.VirtualNode(); v != nil && v.GUID != "" && v.Hostname != "" {
			j.vnodeMu.Lock()
			j.vnode = *v.Copy()
			j.vnodeMu.Unlock()
		}
	}
	tx, err := j.prepareEmission(curTime)
	if err != nil {
		j.retries.Add(1)
		j.Warningf("prepare vnode host info failed: %v", err)
		return
	}
	j.buf.Reset()
	defer func() {
		_ = tx.Abort()
		j.buf.Reset()
	}()
	if tx.processMetrics(metrics, sinceLastRun) {
		j.retries.Store(0)
	} else {
		j.retries.Add(1)
	}
	owner, definition := tx.owner, tx.definition
	if tx.hostBytes == 0 {
		owner = nil
		definition = nil
	}
	if _, err := commitHostOutput(j.out, hostoutput.Request{
		Owner:      owner,
		Definition: definition,
		Payload:    j.buf.Bytes(),
	}, tx); err != nil {
		poisonJobOutput(j.out, err)
		j.Errorf("collection output failed: %v", err)
	}
}

func (j *Job) collect() collectedMetrics {
	j.panicked.Store(false)
	defer func() {
		if r := recover(); r != nil {
			j.panicked.Store(true)
			err := sanitizeLifecycleError(j.lifecycleErrorSanitizer, fmt.Errorf("panic %v", r))
			j.Errorf("PANIC: %v", err)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	var mx collectedMetrics
	mx.intMetrics = j.module.Collect(context.TODO())

	return mx
}

func getChartType(chart *collectorapi.Chart, j *Job) string {
	if chart.CachedType() != "" {
		return chart.CachedType()
	}
	typ := j.FullName()
	if chart.IDSep {
		if i := strings.IndexByte(chart.ID, '.'); i != -1 {
			typ += "_" + chart.ID[:i]
		}
	}
	if chart.OverModule != "" {
		if suffix, ok := strings.CutPrefix(typ, j.ModuleName()); ok {
			typ = chart.OverModule + suffix
		}
	}
	return typ
}

func getChartID(chart *collectorapi.Chart) string {
	if chart.CachedID() != "" {
		return chart.CachedID()
	}
	if chart.IDSep {
		if i := strings.IndexByte(chart.ID, '.'); i != -1 {
			return chart.ID[i+1:]
		}
	}
	return chart.ID
}

func calcSinceLastRun(curTime, prevRun time.Time) int {
	if prevRun.IsZero() {
		return 0
	}
	return int((curTime.UnixNano() - prevRun.UnixNano()) / 1000)
}

func durationTo(duration time.Duration, to time.Duration) int {
	return int(int64(duration) / (int64(to) / int64(time.Nanosecond)))
}

func firstNotEmpty(val1, val2 string) string {
	if val1 != "" {
		return val1
	}
	return val2
}

func handleZero(v int) int {
	if v == 0 {
		return 1
	}
	return v
}

func cleanPluginName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return r.Replace(name)
}

var lblValueReplacer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)
