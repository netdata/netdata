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
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/tickstate"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
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
	PluginName            string
	Name                  string
	ModuleName            string
	FullName              string
	Source                string
	Module                collectorapi.CollectorV1
	Labels                map[string]string
	Out                   io.Writer
	UpdateEvery           int
	AutoDetectEvery       int
	Priority              int
	IsStock               bool
	Vnode                 vnodes.VirtualNode
	VnodeName             string
	VnodeRevision         uint64
	VnodeMetadataRevision uint64
	VnodeLookup           VnodeLookup
	FunctionOnly          bool
}

func NewJob(cfg JobConfig) *Job {
	var buf bytes.Buffer

	if cfg.UpdateEvery == 0 {
		cfg.UpdateEvery = 1
	}

	j := &Job{
		AutoDetectEvery: cfg.AutoDetectEvery,
		AutoDetectTries: infTries,

		pluginName:            cfg.PluginName,
		name:                  cfg.Name,
		moduleName:            cfg.ModuleName,
		fullName:              cfg.FullName,
		updateEvery:           cfg.UpdateEvery,
		priority:              cfg.Priority,
		isStock:               cfg.IsStock,
		functionOnly:          cfg.FunctionOnly,
		module:                cfg.Module,
		labels:                cfg.Labels,
		out:                   cfg.Out,
		collectStatusChart:    newCollectStatusChart(cfg.PluginName),
		collectDurationChart:  newCollectDurationChart(cfg.PluginName),
		stopCtrl:              newStopController(),
		tick:                  make(chan int),
		buf:                   &buf,
		api:                   netdataapi.New(&buf),
		vnode:                 cfg.Vnode,
		vnodeName:             cfg.VnodeName,
		vnodeRevision:         cfg.VnodeRevision,
		vnodeMetadataRevision: cfg.VnodeMetadataRevision,
		vnodeLookup:           cfg.VnodeLookup,
	}

	log := logger.New().With(jobLoggerAttrs(j.ModuleName(), j.Name(), cfg.Source)...)

	j.Logger = log
	if j.module != nil {
		j.module.GetBase().Logger = log
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
	AutoDetectEvery int
	AutoDetectTries int
	priority        int
	labels          map[string]string

	*logger.Logger

	isStock      bool
	functionOnly bool

	module collectorapi.CollectorV1

	// running tracks whether the job's main loop is active (set in Start, cleared in Start's defer)
	running atomic.Bool

	initialized bool
	panicked    atomic.Bool

	collectStatusChart   *collectorapi.Chart
	collectDurationChart *collectorapi.Chart
	charts               *collectorapi.Charts
	tick                 chan int
	out                  io.Writer
	buf                  *bytes.Buffer
	api                  *netdataapi.API

	vnodeCreated bool
	// vnodeMu covers j.vnode against off-goroutine readers (Vnode is
	// called from the manager loop on registered jobs) racing the job
	// goroutine's writes and the pre-Start baseline write.
	vnodeMu               sync.RWMutex
	vnode                 vnodes.VirtualNode
	vnodeName             string
	vnodeRevision         uint64
	vnodeMetadataRevision uint64
	vnodeLookup           VnodeLookup

	retries atomic.Int64
	prevRun time.Time

	stopCtrl stopController

	// moduleCleanup guards the module's Cleanup to exactly once: it is
	// reachable from the detection-failure defer, the main loop's tail, and
	// Cleanup() (detected-but-never-started jobs are disposed through it).
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

// Panicked returns 'panicked' flag value.
func (j *Job) Panicked() bool {
	return j.panicked.Load()
}

// AutoDetectionEvery returns value of AutoDetectEvery.
func (j *Job) AutoDetectionEvery() int {
	return j.AutoDetectEvery
}

// RetryAutoDetection returns whether it is needed to retry autodetection.
func (j *Job) RetryAutoDetection() bool {
	return retryAutoDetection(j.AutoDetectEvery, j.AutoDetectTries)
}

func (j *Job) Configuration() any {
	return j.module.Configuration()
}

func (j *Job) Vnode() vnodes.VirtualNode {
	j.vnodeMu.RLock()
	defer j.vnodeMu.RUnlock()
	return *j.vnode.Copy()
}

// AutoDetection invokes init, check and postCheck. It handles panic.
// ctx flows into the module's Init/Check calls and must be non-nil.
func (j *Job) AutoDetection(ctx context.Context) (err error) {
	return j.autoDetection(ctx, true)
}

// AutoDetectionManaged leaves failure cleanup with the Job Manager factory.
func (j *Job) AutoDetectionManaged(ctx context.Context) (err error) {
	return j.autoDetection(ctx, false)
}

func (j *Job) autoDetection(ctx context.Context, cleanupOnFailure bool) (err error) {
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
		if err != nil && cleanupOnFailure {
			j.cleanupModule()
		}
	}()

	if j.isStock {
		j.Mute()
	}

	if err = j.init(ctx); err != nil {
		j.Errorf("init failed: %v", err)
		j.Unmute()
		if !isRetryableError(err) {
			j.disableAutoDetection()
		}
		return err
	}

	if err = j.check(ctx); err != nil {
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

// SetVnodeSnapshot commits the vnode config directly into the job BEFORE Start:
// registration-time freshness must be visible to Cleanup even when the job
// never collects. Pre-Start only: the job goroutine does not exist yet, and the
// write is published to it by the Start goroutine launch. Module-owned vnode
// state is never overridden.
func (j *Job) SetVnodeSnapshot(snapshot VnodeSnapshot) {
	if snapshot.Vnode == nil || (j.module != nil && j.module.VirtualNode() != nil) {
		return
	}
	next := snapshot.Vnode.Copy()
	j.vnodeMu.Lock()
	j.vnode = *next
	if snapshot.Revision != 0 {
		j.vnodeRevision = snapshot.Revision
	}
	if snapshot.MetadataRevision != 0 {
		j.vnodeMetadataRevision = snapshot.MetadataRevision
	}
	j.vnodeMu.Unlock()
}

func (j *Job) refreshVnodeSnapshot() bool {
	if j.vnodeName == "" || j.vnodeLookup == nil {
		return false
	}
	snapshot, ok := j.vnodeLookup(j.vnodeName)
	if !ok {
		return false
	}
	return j.applyVnodeSnapshot(snapshot)
}

func (j *Job) applyVnodeSnapshot(snapshot VnodeSnapshot) bool {
	if snapshot.Vnode == nil {
		return false
	}
	if snapshot.Revision != 0 {
		j.vnodeMu.Lock()
		stale := snapshot.Revision <= j.vnodeRevision
		j.vnodeMu.Unlock()
		if stale {
			return false
		}
	}
	next := snapshot.Vnode.Copy()

	j.vnodeMu.Lock()
	defer j.vnodeMu.Unlock()

	if snapshot.Revision != 0 && snapshot.Revision <= j.vnodeRevision {
		return false
	}
	metadataChanged := snapshot.MetadataRevision == 0 || snapshot.MetadataRevision != j.vnodeMetadataRevision
	createChart := false
	if metadataChanged {
		j.vnodeCreated = false
		createChart = j.vnode.GUID != next.GUID
	}
	j.vnode = *next
	if snapshot.Revision != 0 {
		j.vnodeRevision = snapshot.Revision
	}
	if snapshot.MetadataRevision != 0 {
		j.vnodeMetadataRevision = snapshot.MetadataRevision
	}
	return createChart
}

// Tick Tick.
func (j *Job) Tick(clock int) {
	enqueueTickWithSkipLog(j.tick, clock, j.functionOnly, j.updateEvery, int(j.retries.Load()), &j.skipTracker, j.Logger)
}

// IsRunning returns true if the job's main loop is currently running.
// This is safe to call from any goroutine.
func (j *Job) IsRunning() bool {
	return j.running.Load()
}

// Module returns the underlying module instance.
// This allows function handlers to access the collector for querying data.
func (j *Job) Module() collectorapi.CollectorV1 {
	return j.module
}

// Collector returns the underlying collector instance bound to this job.
func (j *Job) Collector() any {
	return j.module
}

// IsFunctionOnly returns true if this job is function-only (no metrics collection).
func (j *Job) IsFunctionOnly() bool {
	return j.functionOnly
}

// Start starts job main loop.
func (j *Job) Start() {
	j.run(true, nil)
}

// StartManaged starts the collector loop while leaving Cleanup ownership with
// the caller. It acknowledges readiness only after the loop has published its
// running state.
func (j *Job) StartManaged(ready chan<- struct{}) {
	j.run(false, ready)
}

func (j *Job) run(cleanup bool, ready chan<- struct{}) {
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
	if cleanup {
		j.Cleanup()
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
	disableAutoDetection(&j.AutoDetectEvery)
}

func (j *Job) cleanupModule() {
	j.moduleCleanup.Do(func() { j.module.Cleanup(context.TODO()) })
}

func (j *Job) Cleanup() {
	j.cleanupModule()
	j.buf.Reset()
	if !collectorapi.ShouldObsoleteCharts() {
		return
	}

	// Netdata automatically obsoletes vnode charts when no updates are sent.
	// For virtual nodes with a stale label, we must not send anything:
	//   - Sending a HOST line would incorrectly mark the vnode as active.
	isVnodeWithStaleConfig := j.vnode.Labels["_node_stale_after_seconds"] != ""

	if !isVnodeWithStaleConfig {
		if !j.vnodeCreated && j.vnode.GUID != "" {
			j.sendVnodeHostInfo()
			j.vnodeCreated = true
		}
		j.api.HOST(j.vnode.GUID)

		if j.charts != nil {
			for _, chart := range *j.charts {
				if chart.IsCreated() {
					chart.MarkRemove()
					j.createChart(chart)
				}
			}
		}
	}

	j.api.HOST("")

	if j.collectStatusChart.IsCreated() {
		j.collectStatusChart.MarkRemove()
		j.createChart(j.collectStatusChart)
	}
	if j.collectDurationChart.IsCreated() {
		j.collectDurationChart.MarkRemove()
		j.createChart(j.collectDurationChart)
	}

	if j.buf.Len() > 0 {
		if err := commitJobOutput(j.out, j.buf.Bytes()); err != nil {
			j.Errorf("cleanup output failed: %v", err)
		}
	}
}

// CleanupRejected releases a constructed job without emitting cleanup output.
func (j *Job) CleanupRejected() {
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
		consumeAutoDetectTry(&j.AutoDetectTries)
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
			j.Errorf("charts check: %v", err)
			return err
		}
	}
	return nil
}

func (j *Job) runOnce() {
	defer j.ResetAllOnce()

	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.prevRun = curTime

	metrics := j.collect()

	if j.panicked.Load() {
		return
	}

	if j.processMetrics(metrics, curTime, sinceLastRun) {
		j.retries.Store(0)
	} else {
		j.retries.Add(1)
	}

	if err := commitJobOutput(j.out, j.buf.Bytes()); err != nil {
		j.Errorf("collection output failed: %v", err)
	}
	j.buf.Reset()
}

func (j *Job) collect() collectedMetrics {
	j.panicked.Store(false)
	defer func() {
		if r := recover(); r != nil {
			j.panicked.Store(true)
			j.Errorf("PANIC: %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	var mx collectedMetrics
	mx.intMetrics = j.module.Collect(context.TODO())

	return mx
}

func (j *Job) processMetrics(mx collectedMetrics, startTime time.Time, sinceLastRun int) bool {
	var createChart bool
	if j.module.VirtualNode() == nil {
		createChart = j.refreshVnodeSnapshot()
	}

	if !j.vnodeCreated {
		if j.vnode.GUID == "" {
			if v := j.module.VirtualNode(); v != nil && v.GUID != "" && v.Hostname != "" {
				j.vnodeMu.Lock()
				j.vnode = *v
				j.vnodeMu.Unlock()
			}
		}
		if j.vnode.GUID != "" {
			j.sendVnodeHostInfo()
			j.vnodeCreated = true
		}
	}

	bufLenBeforeHost := j.buf.Len()
	j.api.HOST(j.vnode.GUID)

	elapsed := int64(durationTo(time.Since(startTime), time.Millisecond))

	var i, updated, created int
	for _, chart := range *j.charts {
		if !chart.IsCreated() || createChart {
			typeID := fmt.Sprintf("%s.%s", getChartType(chart, j), getChartID(chart))
			if len(typeID) >= NetdataChartIDMaxLength {
				j.Warningf("chart 'type.id' length (%d) >= max allowed (%d), the chart is ignored (%s)",
					len(typeID), NetdataChartIDMaxLength, typeID)
				chart.SetIgnored(true)
			}
			j.createChart(chart)
			created++
		}
		if chart.IsRemoved() {
			continue
		}
		(*j.charts)[i] = chart
		i++
		if len(mx.intMetrics)+len(mx.floatMetrics) == 0 || chart.Obsolete {
			continue
		}
		if j.updateChart(chart, mx, sinceLastRun) {
			updated++
		}
	}
	*j.charts = (*j.charts)[:i]

	if updated == 0 && created == 0 && j.vnode.GUID != "" {
		j.buf.Truncate(bufLenBeforeHost)
	}

	j.api.HOST("")

	if !j.collectStatusChart.IsCreated() || createChart {
		j.collectStatusChart.ID = fmt.Sprintf("%s_%s_data_collection_status", cleanPluginName(j.pluginName), j.FullName())
		j.createChart(j.collectStatusChart)
	}

	if !j.collectDurationChart.IsCreated() || createChart {
		j.collectDurationChart.ID = fmt.Sprintf("%s_%s_data_collection_duration", cleanPluginName(j.pluginName), j.FullName())
		j.createChart(j.collectDurationChart)
	}

	intMx := collectedMetrics{intMetrics: map[string]int64{"success": oldmetrix.Bool(updated > 0), "failed": oldmetrix.Bool(updated == 0)}}
	j.updateChart(j.collectStatusChart, intMx, sinceLastRun)

	if updated == 0 {
		return false
	}

	intMx = collectedMetrics{intMetrics: map[string]int64{"duration": elapsed}}
	j.updateChart(j.collectDurationChart, intMx, sinceLastRun)

	return true
}

func (j *Job) sendVnodeHostInfo() {
	info, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     j.vnode.GUID,
		Hostname: j.vnode.Hostname,
		Labels:   j.vnode.Labels,
	})
	if err != nil {
		j.Warningf("prepare vnode host info failed: %v", err)
		return
	}

	j.vnodeMu.Lock()
	j.vnode.Hostname = info.Hostname
	j.vnode.Labels = info.Labels
	j.vnodeMu.Unlock()
	j.api.HOSTINFO(info)
}

func (j *Job) createChart(chart *collectorapi.Chart) {
	defer func() { chart.SetCreated(true) }()
	if chart.IsIgnored() {
		return
	}

	if chart.Priority == 0 {
		chart.Priority = j.priority
		j.priority++
	}
	updateEvery := j.updateEvery
	if chart.UpdateEvery > 0 {
		updateEvery = chart.UpdateEvery
	}

	j.api.CHART(netdataapi.ChartOpts{
		TypeID:      getChartType(chart, j),
		ID:          getChartID(chart),
		Name:        chart.OverID,
		Title:       chart.Title,
		Units:       chart.Units,
		Family:      chart.Fam,
		Context:     chart.Ctx,
		ChartType:   chart.Type.String(),
		Priority:    chart.Priority,
		UpdateEvery: updateEvery,
		Options:     chart.Opts.String(),
		Plugin:      j.pluginName,
		Module:      j.moduleName,
	})

	if chart.Obsolete {
		_ = j.api.EMPTYLINE()
		return
	}

	seen := make(map[string]bool)
	for _, l := range chart.Labels {
		if l.Key != "" {
			seen[l.Key] = true
			ls := l.Source
			// the default should be auto
			// https://github.com/netdata/netdata/blob/cc2586de697702f86a3c34e60e23652dd4ddcb42/database/rrd.h#L205
			if ls == 0 {
				ls = collectorapi.LabelSourceAuto
			}
			j.api.CLABEL(l.Key, lblValueReplacer.Replace(l.Value), ls)
		}
	}
	for k, v := range j.labels {
		if !seen[k] {
			j.api.CLABEL(k, lblValueReplacer.Replace(v), collectorapi.LabelSourceConf)
		}
	}
	j.api.CLABEL("_collect_job", lblValueReplacer.Replace(j.Name()), collectorapi.LabelSourceAuto)
	j.api.CLABELCOMMIT()

	for _, dim := range chart.Dims {
		j.api.DIMENSION(netdataapi.DimensionOpts{
			ID:         firstNotEmpty(dim.Name, dim.ID),
			Name:       dim.Name,
			Algorithm:  dim.Algo.String(),
			Multiplier: handleZero(dim.Mul),
			Divisor:    handleZero(dim.Div),
			Options:    dim.DimOpts.String(),
		})
	}
	for _, v := range chart.Vars {
		name := firstNotEmpty(v.Name, v.ID)
		j.api.VARIABLE(name, v.Value)
	}
	_ = j.api.EMPTYLINE()
}

func (j *Job) updateChart(chart *collectorapi.Chart, mx collectedMetrics, sinceLastRun int) bool {
	if chart.IsIgnored() {
		dims := chart.Dims[:0]
		for _, dim := range chart.Dims {
			if !dim.IsRemoved() {
				dims = append(dims, dim)
			}
		}
		chart.Dims = dims
		return false
	}

	// Handle SkipGaps: check if any dimension has data
	if chart.SkipGaps {
		hasData := false
		for _, dim := range chart.Dims {
			if dim.IsRemoved() {
				continue
			}
			if _, hasData = mx.getValue(dim.ID); hasData {
				break
			}
		}
		if !hasData {
			// No dimensions have data - skip this chart entirely
			return false
		}
		// At least one dimension has data - proceed with deltaTime=0
		sinceLastRun = 0
	} else if !chart.IsUpdated() {
		sinceLastRun = 0
	}

	j.api.BEGIN(getChartType(chart, j), getChartID(chart), sinceLastRun)

	var i, updated int
	for _, dim := range chart.Dims {
		if dim.IsRemoved() {
			continue
		}
		chart.Dims[i] = dim
		i++

		name := firstNotEmpty(dim.Name, dim.ID)
		v, ok := mx.getValue(dim.ID)
		if !ok {
			j.api.SETEMPTY(name)
			continue
		}
		updated++
		if dim.Float {
			j.api.SETFLOAT(name, v)
		} else {
			j.api.SET(name, int64(v))
		}
	}

	chart.Dims = chart.Dims[:i]

	for _, vr := range chart.Vars {
		if v, ok := mx.getValue(vr.ID); ok {
			name := firstNotEmpty(vr.Name, vr.ID)
			j.api.VARIABLE(name, v)
		}
	}

	j.api.END()

	chart.SetUpdated(updated > 0)
	if chart.IsUpdated() {
		chart.Retries = 0
	} else {
		chart.Retries++
	}
	return chart.IsUpdated()
}

func (j *Job) penalty() int {
	return penaltyFromRetries(int(j.retries.Load()), j.updateEvery)
}

func getChartType(chart *collectorapi.Chart, j *Job) string {
	if chart.CachedType() != "" {
		return chart.CachedType()
	}
	if !chart.IDSep {
		chart.SetCachedType(j.FullName())
	} else if i := strings.IndexByte(chart.ID, '.'); i != -1 {
		chart.SetCachedType(j.FullName() + "_" + chart.ID[:i])
	} else {
		chart.SetCachedType(j.FullName())
	}
	if chart.OverModule != "" {
		cachedType := chart.CachedType()
		if v, ok := strings.CutPrefix(cachedType, j.ModuleName()); ok {
			chart.SetCachedType(chart.OverModule + v)
		}
	}
	return chart.CachedType()
}

func getChartID(chart *collectorapi.Chart) string {
	if chart.CachedID() != "" {
		return chart.CachedID()
	}
	if !chart.IDSep {
		return chart.ID
	}
	if i := strings.IndexByte(chart.ID, '.'); i != -1 {
		chart.SetCachedID(chart.ID[i+1:])
	} else {
		chart.SetCachedID(chart.ID)
	}
	return chart.CachedID()
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
