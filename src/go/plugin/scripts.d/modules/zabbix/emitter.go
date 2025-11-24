// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	pkgzabbix "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix/jobengine"
	zpre "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbixpreproc"
)

type pipelineEmitter struct {
	log    *logger.Logger
	charts *module.Charts

	mu      sync.Mutex
	jobs    map[string]*jobState
	metrics map[string]int64
}

type jobState struct {
	cfg          pkgzabbix.JobConfig
	engine       *jobengine.Job
	instances    map[string]*instanceBinding
	stateBinding *stateChartBinding
}

type instanceBinding struct {
	id     string
	macros map[string]string
	charts map[string]*pipelineBinding
	state  *stateChartBinding
}

type pipelineBinding struct {
	chart *module.Chart
	dims  map[string]string
}

type metricUpdate struct {
	dimID string
	value int64
}

type stateChartBinding struct {
	chartID string
	dims    map[string]string
}

var stateDimensions = []string{"collect_failure", "lld_failure", "extraction_failure", "dimension_failure", "ok"}

func newPipelineEmitter(log *logger.Logger, proc *zpre.Preprocessor, charts *module.Charts, jobs []pkgzabbix.JobConfig) (*pipelineEmitter, error) {
	states := make(map[string]*jobState, len(jobs))
	for _, cfg := range jobs {
		engine, err := jobengine.NewJob(cfg, proc, jobengine.Options{Logger: log})
		if err != nil {
			return nil, err
		}
		states[cfg.Name] = &jobState{
			cfg:       cfg,
			engine:    engine,
			instances: make(map[string]*instanceBinding),
		}
	}
	return &pipelineEmitter{
		log:     log,
		charts:  charts,
		jobs:    states,
		metrics: make(map[string]int64),
	}, nil
}

func (e *pipelineEmitter) Emit(job runtime.JobRuntime, res runtime.ExecutionResult, snap runtime.JobSnapshot) {
	state := e.getState(job.Spec.Name)
	if state == nil {
		return
	}
	input := jobengine.Input{Payload: res.Output, Timestamp: res.End}
	if res.Err != nil || res.ExitCode != 0 {
		if input.Payload == nil {
			input.Payload = []byte{}
		}
		input.CollectError = fmt.Errorf("collect failed: %w", res.Err)
	}
	result := state.engine.Process(input)
	metricUpdates, stateUpdates := e.prepareUpdates(state.cfg.Name, result)
	for _, upd := range metricUpdates {
		e.setMetric(upd.dimID, upd.value)
	}
	for _, upd := range stateUpdates {
		e.setMetric(upd.dimID, upd.value)
	}
}

func (e *pipelineEmitter) prepareUpdates(jobName string, result jobengine.Result) ([]metricUpdate, []metricUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, ok := e.jobs[jobName]
	if !ok {
		return nil, nil
	}
	for _, inst := range result.Removed {
		if binding, ok := state.instances[inst.ID]; ok {
			e.removeInstanceLocked(state.cfg, binding)
			delete(state.instances, inst.ID)
		}
	}
	for _, info := range result.Active {
		binding := e.ensureInstanceBindingLocked(state, info)
		binding.macros = cloneMacros(info.Macros)
	}
	metricUpdates := e.buildMetricUpdatesLocked(state, result.Metrics)
	stateUpdates := e.buildStateUpdatesLocked(state, result.State)
	return metricUpdates, stateUpdates
}

func (e *pipelineEmitter) ensureInstanceBindingLocked(state *jobState, info jobengine.InstanceInfo) *instanceBinding {
	binding, ok := state.instances[info.ID]
	if !ok {
		binding = &instanceBinding{id: info.ID, macros: make(map[string]string), charts: make(map[string]*pipelineBinding)}
		state.instances[info.ID] = binding
	}
	if binding.charts == nil {
		binding.charts = make(map[string]*pipelineBinding)
	}
	return binding
}

func (e *pipelineEmitter) buildMetricUpdatesLocked(state *jobState, metrics []jobengine.MetricResult) []metricUpdate {
	updates := make([]metricUpdate, 0, len(metrics))
	for _, mr := range metrics {
		inst := state.instances[mr.Instance.ID]
		if inst == nil {
			continue
		}
		binding, err := e.ensurePipelineChartLocked(state.cfg, inst, mr.Pipeline, inst.macros)
		if err != nil {
			e.logWarn("chart registration failed", state.cfg.Name, mr.Pipeline.Name, err)
			continue
		}
		baseName := dimensionBaseName(mr.Pipeline, inst.macros)
		key := dimensionKey(baseName, mr)
		dimensionID, ok := binding.dims[key]
		if !ok {
			chart := binding.chart
			if chart == nil {
				continue
			}
			dimName := dimensionDisplayName(baseName, mr)
			newDim := &module.Dim{ID: dimID(chart.ID, ids.Sanitize(key)), Name: dimName, Algo: dimAlgorithm(mr.Pipeline.Algorithm)}
			if err := chart.AddDim(newDim); err != nil {
				e.logWarn("dimension registration failed", state.cfg.Name, mr.Pipeline.Name, err)
				continue
			}
			dimensionID = newDim.ID
			binding.dims[key] = dimensionID
		}
		updates = append(updates, metricUpdate{dimID: dimensionID, value: scaleValue(mr.Value, mr.Pipeline.Precision)})
	}
	return updates
}

func (e *pipelineEmitter) buildStateUpdatesLocked(state *jobState, summary jobengine.StateSummary) []metricUpdate {
	var updates []metricUpdate
	binding, err := e.ensureJobStateChart(state)
	if err != nil {
		e.logWarn("state chart registration failed", state.cfg.Name, "job_state", err)
	} else {
		updates = append(updates, stateMetricUpdates(binding, summary.Job)...)
	}
	for id, inst := range state.instances {
		flags := summary.Instances[id]
		ib, err := e.ensureInstanceStateChart(state.cfg, inst)
		if err != nil {
			e.logWarn("state chart registration failed", state.cfg.Name, id, err)
			continue
		}
		updates = append(updates, stateMetricUpdates(ib, flags)...)
	}
	return updates
}

func stateMetricUpdates(binding *stateChartBinding, flags jobengine.FailureFlags) []metricUpdate {
	updates := make([]metricUpdate, 0, len(stateDimensions))
	updates = append(updates,
		metricUpdate{dimID: binding.dims["collect_failure"], value: boolToInt(flags.Collect)},
		metricUpdate{dimID: binding.dims["lld_failure"], value: boolToInt(flags.LLD)},
		metricUpdate{dimID: binding.dims["extraction_failure"], value: boolToInt(flags.Extraction)},
		metricUpdate{dimID: binding.dims["dimension_failure"], value: boolToInt(flags.Dimension)},
		metricUpdate{dimID: binding.dims["ok"], value: boolToInt(!flags.Collect && !flags.LLD && !flags.Extraction && !flags.Dimension)},
	)
	return updates
}

func (e *pipelineEmitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for name := range e.jobs {
		e.removeJobLocked(name)
	}
	return nil
}

func (e *pipelineEmitter) RemoveJob(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeJobLocked(name)
}

func (e *pipelineEmitter) removeJobLocked(name string) {
	state, ok := e.jobs[name]
	if !ok {
		return
	}
	if state.stateBinding != nil {
		_ = e.charts.Remove(state.stateBinding.chartID)
		state.stateBinding = nil
	}
	for _, inst := range state.instances {
		e.removeInstanceLocked(state.cfg, inst)
	}
	state.engine.Destroy()
	delete(e.jobs, name)
}

func (e *pipelineEmitter) Flush() map[string]int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.metrics) == 0 {
		return nil
	}
	out := make(map[string]int64, len(e.metrics))
	for k, v := range e.metrics {
		out[k] = v
	}
	e.metrics = make(map[string]int64)
	return out
}

func (e *pipelineEmitter) setMetric(dimID string, value int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics[dimID] = value
}

func (e *pipelineEmitter) getState(name string) *jobState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.jobs[name]
}

func buildChart(job pkgzabbix.JobConfig, inst *instanceBinding, pipe *pkgzabbix.PipelineConfig, macros map[string]string) *module.Chart {
	title := expandTemplate(pipe.Title, macros)
	if title == "" {
		title = expandTemplate(pipe.Context, macros)
	}
	if title == "" {
		title = fmt.Sprintf("%s %s", job.Name, pipe.Name)
	}
	units := expandTemplate(pipe.Unit, macros)
	if units == "" {
		units = "value"
	}
	fam := expandTemplate(pipe.Family, macros)
	if fam == "" {
		fam = expandTemplate(job.LLD.Family, macros)
	}
	if fam == "" {
		fam = job.Name
	}
	ctx := expandTemplate(pipe.Context, macros)
	if ctx == "" {
		ctx = pipe.Name
	}
	sanitizedCtx := ids.Sanitize(ctx)
	if sanitizedCtx == "" {
		sanitizedCtx = ids.Sanitize(pipe.Name)
	}
	dimName := expandTemplate(pipe.Dimension, macros)
	if dimName == "" {
		dimName = pipe.Name
	}
	chartIdentifier := chartID(job, inst.id, sanitizedCtx)
	dimensionID := dimID(chartIdentifier, dimName)
	chart := &module.Chart{
		ID:           chartIdentifier,
		Title:        title,
		Units:        units,
		Fam:          fam,
		Ctx:          fmt.Sprintf("zabbix.%s", sanitizedCtx),
		Type:         chartType(pipe.ChartType),
		TypeOverride: "zabbix",
		Priority:     1000,
		Labels:       buildLabels(job, inst, pipe, macros),
		Dims: module.Dims{
			{
				ID:   dimensionID,
				Name: dimName,
				Algo: dimAlgorithm(pipe.Algorithm),
			},
		},
	}
	return chart
}

func buildJobStateChart(job pkgzabbix.JobConfig) *module.Chart {
	chartID := jobStateChartID(job)
	dims, _ := buildStateChartDims(chartID)
	return &module.Chart{
		ID:           chartID,
		Title:        fmt.Sprintf("Zabbix %s state", job.Name),
		Units:        "state",
		Fam:          job.Name,
		Ctx:          fmt.Sprintf("zabbix.%s.state", ids.Sanitize(job.Name)),
		Type:         module.Line,
		TypeOverride: "zabbix",
		Priority:     800,
		Labels:       jobLabels(job),
		Dims:         dims,
		Opts: module.Opts{
			Detail: true,
		},
	}
}

func buildInstanceStateChart(job pkgzabbix.JobConfig, inst *instanceBinding) *module.Chart {
	chartID := instanceStateChartID(job, inst)
	macros := cloneMacros(inst.macros)
	dims, _ := buildStateChartDims(chartID)
	return &module.Chart{
		ID:           chartID,
		Title:        fmt.Sprintf("Zabbix %s %s state", job.Name, inst.id),
		Units:        "state",
		Fam:          job.Name,
		Ctx:          fmt.Sprintf("zabbix.%s.instance.state", ids.Sanitize(job.Name)),
		Type:         module.Line,
		TypeOverride: "zabbix",
		Priority:     850,
		Labels:       buildLabels(job, inst, nil, macros),
		Dims:         dims,
		Opts: module.Opts{
			Detail: true,
		},
	}
}

func (e *pipelineEmitter) ensureJobStateChart(state *jobState) (*stateChartBinding, error) {
	if state.stateBinding != nil {
		return state.stateBinding, nil
	}
	chart := buildJobStateChart(state.cfg)
	if err := e.charts.Add(chart); err != nil {
		return nil, err
	}
	bind := newStateBinding(chart)
	state.stateBinding = bind
	return bind, nil
}

func (e *pipelineEmitter) ensureInstanceStateChart(job pkgzabbix.JobConfig, inst *instanceBinding) (*stateChartBinding, error) {
	if inst.state != nil {
		return inst.state, nil
	}
	chart := buildInstanceStateChart(job, inst)
	if err := e.charts.Add(chart); err != nil {
		return nil, err
	}
	bind := newStateBinding(chart)
	inst.state = bind
	return bind, nil
}

func newStateBinding(chart *module.Chart) *stateChartBinding {
	bind := &stateChartBinding{chartID: chart.ID, dims: make(map[string]string, len(stateDimensions))}
	for idx, name := range stateDimensions {
		if idx < len(chart.Dims) && chart.Dims[idx] != nil {
			bind.dims[name] = chart.Dims[idx].ID
		}
	}
	return bind
}

func (e *pipelineEmitter) removeInstanceLocked(cfg pkgzabbix.JobConfig, inst *instanceBinding) {
	if inst == nil {
		return
	}
	for _, binding := range inst.charts {
		if binding != nil && binding.chart != nil {
			_ = e.charts.Remove(binding.chart.ID)
		}
	}
	if inst.state != nil {
		_ = e.charts.Remove(inst.state.chartID)
		inst.state = nil
	}
}

func (e *pipelineEmitter) ensurePipelineChartLocked(job pkgzabbix.JobConfig, inst *instanceBinding, pipe *pkgzabbix.PipelineConfig, macros map[string]string) (*pipelineBinding, error) {
	binding := inst.charts[pipe.Name]
	if binding != nil {
		return binding, nil
	}
	chart := buildChart(job, inst, pipe, macros)
	if err := e.charts.Add(chart); err != nil {
		return nil, err
	}
	binding = &pipelineBinding{chart: chart, dims: make(map[string]string)}
	base := dimensionBaseName(pipe, macros)
	if len(chart.Dims) > 0 && chart.Dims[0] != nil {
		binding.dims[dimensionKey(base, jobengine.MetricResult{})] = chart.Dims[0].ID
	}
	inst.charts[pipe.Name] = binding
	return binding, nil
}

func buildStateChartDims(chartID string) (module.Dims, map[string]string) {
	dims := make(module.Dims, 0, len(stateDimensions))
	idMap := make(map[string]string, len(stateDimensions))
	for _, name := range stateDimensions {
		dimID := fmt.Sprintf("%s.%s", chartID, ids.Sanitize(name))
		dims = append(dims, &module.Dim{ID: dimID, Name: name, Algo: module.Absolute})
		idMap[name] = dimID
	}
	return dims, idMap
}

func jobStateChartID(job pkgzabbix.JobConfig) string {
	return fmt.Sprintf("zabbix.%s.job.state", ids.Sanitize(job.Name))
}

func instanceStateChartID(job pkgzabbix.JobConfig, inst *instanceBinding) string {
	return fmt.Sprintf("zabbix.%s.%s.state", ids.Sanitize(job.Name), inst.id)
}

func buildLabels(job pkgzabbix.JobConfig, inst *instanceBinding, pipe *pkgzabbix.PipelineConfig, macros map[string]string) []module.Label {
	labels := make([]module.Label, 0, len(job.LLD.Labels)+5)
	labels = append(labels, jobLabels(job)...)
	for key, tmpl := range job.LLD.Labels {
		val := expandTemplate(tmpl, macros)
		if val == "" {
			continue
		}
		labels = append(labels, module.Label{Key: key, Value: val, Source: module.LabelSourceConf})
	}
	instanceLabel := inst.id
	if macroID := macros["{#INSTANCE_ID}"]; macroID != "" {
		instanceLabel = macroID
	}
	labels = append(labels,
		module.Label{Key: "instance", Value: inst.id, Source: module.LabelSourceConf},
		module.Label{Key: "zabbix_instance", Value: instanceLabel, Source: module.LabelSourceConf},
	)
	if pipe != nil && pipe.Name != "" {
		labels = append(labels, module.Label{Key: "zabbix_pipeline", Value: pipe.Name, Source: module.LabelSourceConf})
	}
	return labels
}

func jobLabels(job pkgzabbix.JobConfig) []module.Label {
	labels := []module.Label{
		{Key: "zabbix_job", Value: job.Name, Source: module.LabelSourceConf},
	}
	scheduler := strings.TrimSpace(job.Scheduler)
	if scheduler != "" {
		labels = append(labels, module.Label{Key: "zabbix_scheduler", Value: scheduler, Source: module.LabelSourceConf})
	}
	if job.Vnode != "" {
		labels = append(labels, module.Label{Key: "zabbix_vnode", Value: job.Vnode, Source: module.LabelSourceConf})
	}
	collection := strings.TrimSpace(string(job.Collection.Type))
	if collection != "" {
		labels = append(labels, module.Label{Key: "zabbix_collection", Value: collection, Source: module.LabelSourceConf})
	}
	return labels
}

func chartID(job pkgzabbix.JobConfig, instanceID, ctx string) string {
	return fmt.Sprintf("zabbix.%s.%s.%s", ids.Sanitize(job.Name), instanceID, ctx)
}

func dimID(chartID, dimension string) string {
	return fmt.Sprintf("%s.%s", chartID, ids.Sanitize(dimension))
}

func dimensionBaseName(pipe *pkgzabbix.PipelineConfig, macros map[string]string) string {
	name := expandTemplate(pipe.Dimension, macros)
	if strings.TrimSpace(name) == "" {
		name = pipe.Name
	}
	if strings.TrimSpace(name) == "" {
		name = "value"
	}
	return name
}

func dimensionKey(base string, metric jobengine.MetricResult) string {
	parts := []string{base}
	if metric.Name != "" {
		parts = append(parts, metric.Name)
	}
	if len(metric.Labels) > 0 {
		keys := make([]string, 0, len(metric.Labels))
		for k := range metric.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, metric.Labels[k]))
		}
	}
	return strings.Join(parts, "|")
}

func dimensionDisplayName(base string, metric jobengine.MetricResult) string {
	suffix := []string{}
	if metric.Name != "" {
		suffix = append(suffix, metric.Name)
	}
	if len(metric.Labels) > 0 {
		keys := make([]string, 0, len(metric.Labels))
		for k := range metric.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, metric.Labels[k]))
		}
		suffix = append(suffix, strings.Join(pairs, ","))
	}
	if len(suffix) == 0 {
		return base
	}
	return fmt.Sprintf("%s (%s)", base, strings.Join(suffix, " | "))
}

func expandTemplate(tmpl string, macros map[string]string) string {
	if tmpl == "" || len(macros) == 0 {
		return tmpl
	}
	res := tmpl
	for k, v := range macros {
		res = strings.ReplaceAll(res, k, v)
	}
	return res
}

func buildInstanceID(cfg pkgzabbix.JobConfig, macros map[string]string) string {
	template := cfg.LLD.InstanceTemplate
	if strings.TrimSpace(template) == "" {
		if val, ok := macros["{#INSTANCE_ID}"]; ok {
			template = val
		} else {
			template = fmt.Sprintf("%s_instance", cfg.Name)
		}
	}
	raw := expandTemplate(template, macros)
	if strings.TrimSpace(raw) == "" {
		raw = fmt.Sprintf("%s_instance", cfg.Name)
	}
	return ids.Sanitize(raw)
}

func cloneMacros(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func itemID(cfg pkgzabbix.JobConfig, instanceID, pipe string) string {
	return fmt.Sprintf("%s.%s.%s", cfg.Name, instanceID, pipe)
}

func scaleValue(val float64, precision int) int64 {
	if precision < 0 {
		precision = 0
	}
	factor := math.Pow10(precision)
	return int64(math.Round(val * factor))
}

func dimAlgorithm(algo string) module.DimAlgo {
	switch strings.ToLower(algo) {
	case "incremental":
		return module.Incremental
	case "percentage", "percentage-of-absolute-row":
		return module.PercentOfAbsolute
	case "percentage-of-incremental-row":
		return module.PercentOfIncremental
	default:
		return module.Absolute
	}
}

func chartType(kind string) module.ChartType {
	switch strings.ToLower(kind) {
	case "area":
		return module.Area
	case "stacked":
		return module.Stacked
	default:
		return module.Line
	}
}

func (e *pipelineEmitter) logWarn(msg, job, pipe string, err error) {
	if e.log == nil {
		return
	}
	e.log.Warningf("zabbix %s job=%s pipeline=%s error=%v", msg, job, pipe, err)
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
