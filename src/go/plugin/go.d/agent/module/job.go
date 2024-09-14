// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/netdataapi"
)

var obsoleteLock = &sync.Mutex{}
var obsoleteCharts = true

func DontObsoleteCharts() {
	obsoleteLock.Lock()
	obsoleteCharts = false
	obsoleteLock.Unlock()
}

func shouldObsoleteCharts() bool {
	obsoleteLock.Lock()
	defer obsoleteLock.Unlock()
	return obsoleteCharts
}

var reSpace = regexp.MustCompile(`\s+`)

var ndInternalMonitoringDisabled = os.Getenv("NETDATA_INTERNALS_MONITORING") == "NO"

func newRuntimeChart(pluginName string) *Chart {
	// this is needed to keep the same name as we had before https://github.com/netdata/netdata/go/plugins/plugin/go.d/issues/650
	ctxName := pluginName
	if ctxName == "go.d" {
		ctxName = "go"
	}
	ctxName = reSpace.ReplaceAllString(ctxName, "_")
	return &Chart{
		typ:      "netdata",
		Title:    "Execution time",
		Units:    "ms",
		Fam:      pluginName,
		Ctx:      fmt.Sprintf("netdata.%s_plugin_execution_time", ctxName),
		Priority: 145000,
		Dims: Dims{
			{ID: "time"},
		},
	}
}

type JobConfig struct {
	PluginName      string
	Name            string
	ModuleName      string
	FullName        string
	Module          Module
	Labels          map[string]string
	Out             io.Writer
	UpdateEvery     int
	AutoDetectEvery int
	Priority        int
	IsStock         bool

	VnodeGUID     string
	VnodeHostname string
	VnodeLabels   map[string]string
}

const (
	penaltyStep = 5
	maxPenalty  = 600
	infTries    = -1
)

func NewJob(cfg JobConfig) *Job {
	var buf bytes.Buffer

	if cfg.UpdateEvery == 0 {
		cfg.UpdateEvery = 1
	}

	j := &Job{
		AutoDetectEvery: cfg.AutoDetectEvery,
		AutoDetectTries: infTries,

		pluginName:  cfg.PluginName,
		name:        cfg.Name,
		moduleName:  cfg.ModuleName,
		fullName:    cfg.FullName,
		updateEvery: cfg.UpdateEvery,
		priority:    cfg.Priority,
		isStock:     cfg.IsStock,
		module:      cfg.Module,
		labels:      cfg.Labels,
		out:         cfg.Out,
		runChart:    newRuntimeChart(cfg.PluginName),
		stop:        make(chan struct{}),
		tick:        make(chan int),
		buf:         &buf,
		api:         netdataapi.New(&buf),

		vnodeGUID:     cfg.VnodeGUID,
		vnodeHostname: cfg.VnodeHostname,
		vnodeLabels:   cfg.VnodeLabels,
	}

	log := logger.New().With(
		slog.String("collector", j.ModuleName()),
		slog.String("job", j.Name()),
	)

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

	isStock bool

	module Module

	initialized bool
	panicked    bool

	runChart *Chart
	charts   *Charts
	tick     chan int
	out      io.Writer
	buf      *bytes.Buffer
	api      *netdataapi.API

	retries int
	prevRun time.Time

	stop chan struct{}

	vnodeCreated  bool
	vnodeGUID     string
	vnodeHostname string
	vnodeLabels   map[string]string
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
	return j.panicked
}

// AutoDetectionEvery returns value of AutoDetectEvery.
func (j *Job) AutoDetectionEvery() int {
	return j.AutoDetectEvery
}

// RetryAutoDetection returns whether it is needed to retry autodetection.
func (j *Job) RetryAutoDetection() bool {
	return j.AutoDetectEvery > 0 && (j.AutoDetectTries == infTries || j.AutoDetectTries > 0)
}

func (j *Job) Configuration() any {
	return j.module.Configuration()
}

// AutoDetection invokes init, check and postCheck. It handles panic.
func (j *Job) AutoDetection() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic %v", err)
			j.panicked = true
			j.disableAutoDetection()

			j.Errorf("PANIC %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
		if err != nil {
			j.module.Cleanup()
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

// Tick Tick.
func (j *Job) Tick(clock int) {
	select {
	case j.tick <- clock:
	default:
		j.Debug("skip the tick due to previous run hasn't been finished")
	}
}

// Start starts job main loop.
func (j *Job) Start() {
	j.Infof("started, data collection interval %ds", j.updateEvery)
	defer func() { j.Info("stopped") }()

LOOP:
	for {
		select {
		case <-j.stop:
			break LOOP
		case t := <-j.tick:
			if t%(j.updateEvery+j.penalty()) == 0 {
				j.runOnce()
			}
		}
	}
	j.module.Cleanup()
	j.Cleanup()
	j.stop <- struct{}{}
}

// Stop stops job main loop. It blocks until the job is stopped.
func (j *Job) Stop() {
	// TODO: should have blocking and non blocking stop
	j.stop <- struct{}{}
	<-j.stop
}

func (j *Job) disableAutoDetection() {
	j.AutoDetectEvery = 0
}

func (j *Job) Cleanup() {
	j.buf.Reset()
	if !shouldObsoleteCharts() {
		return
	}

	if !j.vnodeCreated && j.vnodeGUID != "" {
		_ = j.api.HOSTINFO(j.vnodeGUID, j.vnodeHostname, j.vnodeLabels)
		j.vnodeCreated = true
	}
	_ = j.api.HOST(j.vnodeGUID)

	if j.runChart.created {
		j.runChart.MarkRemove()
		j.createChart(j.runChart)
	}
	if j.charts != nil {
		for _, chart := range *j.charts {
			if chart.created {
				chart.MarkRemove()
				j.createChart(chart)
			}
		}
	}

	if j.buf.Len() > 0 {
		_, _ = io.Copy(j.out, j.buf)
	}
}

func (j *Job) init() error {
	if j.initialized {
		return nil
	}

	if err := j.module.Init(); err != nil {
		return err
	}

	j.initialized = true

	return nil
}

func (j *Job) check() error {
	if err := j.module.Check(); err != nil {
		if j.AutoDetectTries != infTries {
			j.AutoDetectTries--
		}
		return err
	}
	return nil
}

func (j *Job) postCheck() error {
	if j.charts = j.module.Charts(); j.charts == nil {
		j.Error("nil charts")
		return errors.New("nil charts")
	}
	if err := checkCharts(*j.charts...); err != nil {
		j.Errorf("charts check: %v", err)
		return err
	}
	return nil
}

func (j *Job) runOnce() {
	curTime := time.Now()
	sinceLastRun := calcSinceLastRun(curTime, j.prevRun)
	j.prevRun = curTime

	metrics := j.collect()

	if j.panicked {
		return
	}

	if j.processMetrics(metrics, curTime, sinceLastRun) {
		j.retries = 0
	} else {
		j.retries++
	}

	_, _ = io.Copy(j.out, j.buf)
	j.buf.Reset()
}

func (j *Job) collect() (result map[string]int64) {
	j.panicked = false
	defer func() {
		if r := recover(); r != nil {
			j.panicked = true
			j.Errorf("PANIC: %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				j.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()
	return j.module.Collect()
}

func (j *Job) processMetrics(metrics map[string]int64, startTime time.Time, sinceLastRun int) bool {
	if !j.vnodeCreated {
		if j.vnodeGUID == "" {
			if v := j.module.VirtualNode(); v != nil && v.GUID != "" && v.Hostname != "" {
				j.vnodeGUID = v.GUID
				j.vnodeHostname = v.Hostname
				j.vnodeLabels = v.Labels
			}
		}
		if j.vnodeGUID != "" {
			_ = j.api.HOSTINFO(j.vnodeGUID, j.vnodeHostname, j.vnodeLabels)
			j.vnodeCreated = true
		}
	}

	_ = j.api.HOST(j.vnodeGUID)

	if !ndInternalMonitoringDisabled && !j.runChart.created {
		j.runChart.ID = fmt.Sprintf("execution_time_of_%s", j.FullName())
		j.createChart(j.runChart)
	}

	elapsed := int64(durationTo(time.Since(startTime), time.Millisecond))

	var i, updated int
	for _, chart := range *j.charts {
		if !chart.created {
			typeID := fmt.Sprintf("%s.%s", j.FullName(), chart.ID)
			if len(typeID) >= NetdataChartIDMaxLength {
				j.Warningf("chart 'type.id' length (%d) >= max allowed (%d), the chart is ignored (%s)",
					len(typeID), NetdataChartIDMaxLength, typeID)
				chart.ignore = true
			}
			j.createChart(chart)
		}
		if chart.remove {
			continue
		}
		(*j.charts)[i] = chart
		i++
		if len(metrics) == 0 || chart.Obsolete {
			continue
		}
		if j.updateChart(chart, metrics, sinceLastRun) {
			updated++
		}
	}
	*j.charts = (*j.charts)[:i]

	if updated == 0 {
		return false
	}
	if !ndInternalMonitoringDisabled {
		j.updateChart(j.runChart, map[string]int64{"time": elapsed}, sinceLastRun)
	}

	return true
}

func (j *Job) createChart(chart *Chart) {
	defer func() { chart.created = true }()
	if chart.ignore {
		return
	}

	if chart.Priority == 0 {
		chart.Priority = j.priority
		j.priority++
	}
	_ = j.api.CHART(
		getChartType(chart, j),
		getChartID(chart),
		chart.OverID,
		chart.Title,
		chart.Units,
		chart.Fam,
		chart.Ctx,
		chart.Type.String(),
		chart.Priority,
		j.updateEvery,
		chart.Opts.String(),
		j.pluginName,
		j.moduleName,
	)

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
				ls = LabelSourceAuto
			}
			_ = j.api.CLABEL(l.Key, lblReplacer.Replace(l.Value), ls)
		}
	}
	for k, v := range j.labels {
		if !seen[k] {
			_ = j.api.CLABEL(k, lblReplacer.Replace(v), LabelSourceConf)
		}
	}
	_ = j.api.CLABEL("_collect_job", lblReplacer.Replace(j.Name()), LabelSourceAuto)
	_ = j.api.CLABELCOMMIT()

	for _, dim := range chart.Dims {
		_ = j.api.DIMENSION(
			firstNotEmpty(dim.Name, dim.ID),
			dim.Name,
			dim.Algo.String(),
			handleZero(dim.Mul),
			handleZero(dim.Div),
			dim.DimOpts.String(),
		)
	}
	for _, v := range chart.Vars {
		if v.Name != "" {
			_ = j.api.VARIABLE(v.Name, v.Value)
		} else {
			_ = j.api.VARIABLE(v.ID, v.Value)
		}
	}
	_ = j.api.EMPTYLINE()
}

func (j *Job) updateChart(chart *Chart, collected map[string]int64, sinceLastRun int) bool {
	if chart.ignore {
		dims := chart.Dims[:0]
		for _, dim := range chart.Dims {
			if !dim.remove {
				dims = append(dims, dim)
			}
		}
		chart.Dims = dims
		return false
	}

	if !chart.updated {
		sinceLastRun = 0
	}

	_ = j.api.BEGIN(
		getChartType(chart, j),
		getChartID(chart),
		sinceLastRun,
	)
	var i, updated int
	for _, dim := range chart.Dims {
		if dim.remove {
			continue
		}
		chart.Dims[i] = dim
		i++
		if v, ok := collected[dim.ID]; !ok {
			_ = j.api.SETEMPTY(firstNotEmpty(dim.Name, dim.ID))
		} else {
			_ = j.api.SET(firstNotEmpty(dim.Name, dim.ID), v)
			updated++
		}
	}
	chart.Dims = chart.Dims[:i]

	for _, vr := range chart.Vars {
		if v, ok := collected[vr.ID]; ok {
			if vr.Name != "" {
				_ = j.api.VARIABLE(vr.Name, v)
			} else {
				_ = j.api.VARIABLE(vr.ID, v)
			}
		}

	}
	_ = j.api.END()

	if chart.updated = updated > 0; chart.updated {
		chart.Retries = 0
	} else {
		chart.Retries++
	}
	return chart.updated
}

func (j *Job) penalty() int {
	v := j.retries / penaltyStep * penaltyStep * j.updateEvery / 2
	if v > maxPenalty {
		return maxPenalty
	}
	return v
}

func getChartType(chart *Chart, j *Job) string {
	if chart.typ != "" {
		return chart.typ
	}
	if !chart.IDSep {
		chart.typ = j.FullName()
	} else if i := strings.IndexByte(chart.ID, '.'); i != -1 {
		chart.typ = j.FullName() + "_" + chart.ID[:i]
	} else {
		chart.typ = j.FullName()
	}
	if chart.OverModule != "" {
		if v := strings.TrimPrefix(chart.typ, j.ModuleName()); v != chart.typ {
			chart.typ = chart.OverModule + v
		}
	}
	return chart.typ
}

func getChartID(chart *Chart) string {
	if chart.id != "" {
		return chart.id
	}
	if !chart.IDSep {
		return chart.ID
	}
	if i := strings.IndexByte(chart.ID, '.'); i != -1 {
		chart.id = chart.ID[i+1:]
	} else {
		chart.id = chart.ID
	}
	return chart.id
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

var lblReplacer = strings.NewReplacer("'", "")
