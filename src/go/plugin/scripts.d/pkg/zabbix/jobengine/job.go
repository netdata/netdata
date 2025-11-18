// SPDX-License-Identifier: GPL-3.0-or-later

package jobengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	pkgzabbix "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
)

// Job orchestrates collection -> LLD -> dependent pipeline execution for a single
// Zabbix-style definition. It wraps the shared Preprocessor and maintains the
// discovered instance catalog between iterations.
type Job struct {
	cfg           pkgzabbix.JobConfig
	proc          *zpre.Preprocessor
	log           *logger.Logger
	mu            sync.RWMutex
	lastDiscovery time.Time
	instances     map[string]*instanceState
}

// Options configure the job engine behavior.
type Options struct {
	Logger *logger.Logger
}

// Input encapsulates the payload passed to Process.
type Input struct {
	Payload      []byte
	Timestamp    time.Time
	CollectError error
}

// Result aggregates the metrics, discovery changes, and failure state after a
// Process call.
type Result struct {
	Job       string
	Timestamp time.Time
	Metrics   []MetricResult
	Active    []InstanceInfo
	Removed   []InstanceInfo
	State     StateSummary
}

// StateSummary tracks failure flags for the job and individual instances.
type StateSummary struct {
	Job       FailureFlags
	Instances map[string]FailureFlags
}

// FailureFlags capture the distinct failure modes per iteration.
type FailureFlags struct {
	Collect    bool
	LLD        bool
	Extraction bool
	Dimension  bool
}

// MetricResult describes a single dependent pipeline outcome.
type MetricResult struct {
	Instance  InstanceInfo
	Pipeline  *pkgzabbix.PipelineConfig
	Value     float64
	Precision int
	Discarded bool
	Error     error
	Name      string
	Labels    map[string]string
}

// InstanceInfo carries the instance ID and macros used for templating.
type InstanceInfo struct {
	ID     string
	Macros map[string]string
}

// NewJob validates the configuration and constructs a Job bound to the given
// Preprocessor.
func NewJob(cfg pkgzabbix.JobConfig, proc *zpre.Preprocessor, opts Options) (*Job, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if proc == nil {
		return nil, errors.New("jobengine: preprocessor is required")
	}
	log := opts.Logger
	if log == nil {
		log = logger.New().With("component", "zabbix/jobengine")
	}
	job := &Job{
		cfg:       cfg,
		proc:      proc,
		log:       log,
		instances: make(map[string]*instanceState),
	}
	return job, nil
}

// Process executes LLD (when configured) and dependent pipelines using the
// provided payload. The returned result describes all emitted metrics alongside
// failure flags for the job and each instance.
func (j *Job) Process(in Input) Result {
	j.mu.Lock()
	defer j.mu.Unlock()

	res := Result{
		Job:       j.cfg.Name,
		Timestamp: in.Timestamp,
		State:     StateSummary{Instances: make(map[string]FailureFlags)},
	}

	if in.Timestamp.IsZero() {
		in.Timestamp = time.Now()
		res.Timestamp = in.Timestamp
	}

	tracker := newIterationState()

	if in.CollectError != nil {
		j.ensureDefaultInstance()
		tracker.setCollectFailureAll(j.instances)
		var active []InstanceInfo
		for _, inst := range j.instances {
			active = append(active, inst.info())
		}
		return j.finalizeResult(tracker, active, nil, res)
	}

	payloadValue := zpre.Value{Data: string(in.Payload), Type: zpre.ValueTypeStr, Timestamp: in.Timestamp}

	var discovery []map[string]string
	var discErr error
	if len(j.cfg.LLD.Steps) > 0 && j.shouldDiscover(in.Timestamp) {
		discovery, discErr = j.runDiscovery(payloadValue)
	}

	var removed []InstanceInfo
	var active []InstanceInfo
	if discErr != nil {
		tracker.setLLDFailureAll(j.instances)
	} else if discovery != nil {
		var applyErr error
		removed, applyErr = j.applyDiscovery(discovery)
		if applyErr != nil {
			tracker.setLLDFailureAll(j.instances)
			discErr = applyErr
		} else {
			j.lastDiscovery = in.Timestamp
		}
	}
	if len(j.cfg.LLD.Steps) == 0 {
		j.ensureDefaultInstance()
	}

	for _, inst := range j.instances {
		active = append(active, inst.info())
		tracker.ensureInstance(inst.id)
	}

	var metrics []MetricResult
	if discErr == nil {
		metrics = j.runPipelines(payloadValue, tracker)
	}

	return j.finalizeResult(tracker, active, removed, Result{
		Job:       res.Job,
		Timestamp: res.Timestamp,
		Metrics:   metrics,
		Active:    active,
		Removed:   removed,
	})
}

// Destroy removes the job-specific state from the shared preprocessor and
// forgets all LLD instances.
func (j *Job) Destroy() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for id := range j.instances {
		for _, pipe := range j.cfg.Pipelines {
			j.proc.ClearState(itemID(j.cfg, id, pipe.Name))
		}
	}
	j.proc.ClearState(fmt.Sprintf("%s.__lld", j.cfg.Name))
	j.instances = make(map[string]*instanceState)
}

func (j *Job) finalizeResult(tracker *iterationState, active, removed []InstanceInfo, res Result) Result {
	res.Active = active
	res.Removed = removed
	res.State.Job = tracker.jobFailure
	res.State.Instances = make(map[string]FailureFlags, len(tracker.instances))
	for id, flags := range tracker.instances {
		res.State.Instances[id] = *flags
	}
	return res
}

func (j *Job) shouldDiscover(now time.Time) bool {
	interval := j.cfg.LLD.IntervalDuration()
	if interval <= 0 {
		return true
	}
	return now.Sub(j.lastDiscovery) >= interval
}

func (j *Job) runDiscovery(val zpre.Value) ([]map[string]string, error) {
	steps := duplicateSteps(j.cfg.LLD.Steps, nil)
	itemID := fmt.Sprintf("%s.__lld", j.cfg.Name)
	res, err := j.proc.ExecutePipeline(itemID, val, steps)
	if err != nil {
		return nil, err
	}
	if len(res.Metrics) == 0 {
		return nil, fmt.Errorf("lld returned no metrics")
	}
	entries := []map[string]string{}
	for _, metric := range res.Metrics {
		payload := strings.TrimSpace(metric.Value)
		if payload == "" {
			continue
		}
		var batch []map[string]string
		if err := json.Unmarshal([]byte(payload), &batch); err != nil {
			return nil, err
		}
		entries = append(entries, batch...)
	}
	return entries, nil
}

func (j *Job) applyDiscovery(entries []map[string]string) ([]InstanceInfo, error) {
	seen := make(map[string]struct{}, len(entries))
	updates := make(map[string]map[string]string, len(entries))
	for _, entry := range entries {
		macros := cloneMacros(entry)
		instanceID := buildInstanceID(j.cfg, macros)
		if instanceID == "" {
			continue
		}
		if _, ok := seen[instanceID]; ok {
			j.warnf("duplicate LLD instance id %q dropped", instanceID)
			return nil, fmt.Errorf("duplicate instance id %s", instanceID)
		}
		seen[instanceID] = struct{}{}
		macros["{#INSTANCE_ID}"] = instanceID
		updates[instanceID] = macros
	}
	for id, macros := range updates {
		inst := j.instances[id]
		if inst == nil {
			inst = &instanceState{id: id, macros: make(map[string]string)}
			j.instances[id] = inst
		}
		inst.macros = cloneMacros(macros)
		inst.missing = 0
	}
	var removed []InstanceInfo
	for id, inst := range j.instances {
		if _, ok := seen[id]; ok {
			continue
		}
		inst.missing++
		threshold := j.cfg.LLD.MaxMissing
		if threshold <= 0 || inst.missing >= threshold {
			removed = append(removed, inst.info())
			j.removeInstance(inst)
			delete(j.instances, id)
		}
	}
	return removed, nil
}

func (j *Job) ensureDefaultInstance() {
	if len(j.instances) > 0 {
		return
	}
	j.instances["default"] = &instanceState{
		id:     "default",
		macros: map[string]string{"{#INSTANCE_ID}": "default"},
	}
}

func (j *Job) removeInstance(inst *instanceState) {
	for _, pipe := range j.cfg.Pipelines {
		j.proc.ClearState(itemID(j.cfg, inst.id, pipe.Name))
	}
}

func (j *Job) runPipelines(val zpre.Value, tracker *iterationState) []MetricResult {
	var out []MetricResult
	for _, inst := range j.instances {
		macros := cloneMacros(inst.macros)
		for idx := range j.cfg.Pipelines {
			pipe := &j.cfg.Pipelines[idx]
			steps := duplicateSteps(pipe.Steps, macros)
			res, err := j.proc.ExecutePipeline(itemID(j.cfg, inst.id, pipe.Name), val, steps)
			if err != nil {
				if isDimensionError(err) {
					tracker.markDimensionFailure(inst.id)
				} else {
					tracker.markExtractionFailure(inst.id)
				}
				out = append(out, MetricResult{Instance: inst.info(), Pipeline: pipe, Error: err})
				continue
			}
			if res.Discarded || len(res.Metrics) == 0 {
				tracker.markDimensionFailure(inst.id)
				out = append(out, MetricResult{Instance: inst.info(), Pipeline: pipe, Discarded: true})
				continue
			}
			if len(res.Metrics) != 1 {
				tracker.markDimensionFailure(inst.id)
				if j.log != nil {
					j.log.Warningf("zabbix: job %s pipeline %s returned %d metrics (expected 1); dropping output", j.cfg.Name, pipe.Name, len(res.Metrics))
				}
				out = append(out, MetricResult{Instance: inst.info(), Pipeline: pipe, Error: fmt.Errorf("pipeline '%s' produced %d metrics; expected 1", pipe.Name, len(res.Metrics))})
				continue
			}
			metric := res.Metrics[0]
			valNum, err := strconv.ParseFloat(strings.TrimSpace(metric.Value), 64)
			if err != nil {
				tracker.markExtractionFailure(inst.id)
				if j.log != nil {
					j.log.Warningf("zabbix: job %s pipeline %s returned non-numeric value %q: %v", j.cfg.Name, pipe.Name, metric.Value, err)
				}
				out = append(out, MetricResult{Instance: inst.info(), Pipeline: pipe, Error: err})
				continue
			}
			out = append(out, MetricResult{
				Instance:  inst.info(),
				Pipeline:  pipe,
				Value:     valNum,
				Precision: pipe.Precision,
				Name:      metric.Name,
				Labels:    cloneLabels(metric.Labels),
			})
		}
	}
	return out
}

// iteration tracking -------------------------------------------------------

type iterationState struct {
	jobFailure FailureFlags
	instances  map[string]*FailureFlags
}

func newIterationState() *iterationState {
	return &iterationState{instances: make(map[string]*FailureFlags)}
}

func (s *iterationState) ensureInstance(id string) {
	if _, ok := s.instances[id]; !ok {
		s.instances[id] = &FailureFlags{}
	}
}

func (s *iterationState) setCollectFailureAll(instances map[string]*instanceState) {
	s.jobFailure.Collect = true
	for id := range instances {
		s.ensureInstance(id)
		s.instances[id].Collect = true
	}
}

func (s *iterationState) setLLDFailureAll(instances map[string]*instanceState) {
	s.jobFailure.LLD = true
	for id := range instances {
		s.ensureInstance(id)
		s.instances[id].LLD = true
	}
}

func (s *iterationState) markExtractionFailure(id string) {
	s.jobFailure.Extraction = true
	s.ensureInstance(id)
	s.instances[id].Extraction = true
}

func (s *iterationState) markDimensionFailure(id string) {
	s.jobFailure.Dimension = true
	s.ensureInstance(id)
	s.instances[id].Dimension = true
}

func isDimensionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "out of range") {
		return true
	}
	if strings.Contains(msg, "does not match") {
		return true
	}
	return false
}

// helpers -----------------------------------------------------------------

type instanceState struct {
	id      string
	macros  map[string]string
	missing int
}

func (inst *instanceState) info() InstanceInfo {
	return InstanceInfo{ID: inst.id, Macros: cloneMacros(inst.macros)}
}

func duplicateSteps(steps []zpre.Step, macros map[string]string) []zpre.Step {
	out := make([]zpre.Step, len(steps))
	for i := range steps {
		step := steps[i]
		if macros != nil {
			step.Params = expandTemplate(step.Params, macros)
		}
		out[i] = step
	}
	return out
}

func itemID(cfg pkgzabbix.JobConfig, instanceID, pipe string) string {
	return fmt.Sprintf("%s.%s.%s", cfg.Name, instanceID, pipe)
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

func cloneMacros(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneLabels(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func buildInstanceID(cfg pkgzabbix.JobConfig, macros map[string]string) string {
	if val, ok := macros["{#INSTANCE_ID}"]; ok {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			return ids.Sanitize(trimmed)
		}
	}
	template := cfg.LLD.InstanceTemplate
	if strings.TrimSpace(template) == "" {
		template = fmt.Sprintf("%s_instance", cfg.Name)
	}
	raw := expandTemplate(template, macros)
	if strings.TrimSpace(raw) == "" {
		raw = fmt.Sprintf("%s_instance", cfg.Name)
	}
	return ids.Sanitize(raw)
}

func (j *Job) warnf(format string, args ...interface{}) {
	if j.log == nil {
		return
	}
	j.log.Warningf(format, args...)
}
