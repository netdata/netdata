// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/units"
)

// SchedulerConfig drives the construction of the async scheduler/executor pair.
type SchedulerConfig struct {
	Logger           *logger.Logger
	Jobs             []spec.JobSpec
	Workers          int
	Shard            string
	Periods          *timeperiod.Set
	UserMacros       map[string]string
	VnodeLookup      func(spec.JobSpec) VnodeInfo
	Emitter          ResultEmitter
	RegisterPerfdata func(spec.JobSpec, output.PerfDatum)
}

// Scheduler wires the per-job timers, executor, and result collection loops.
type Scheduler struct {
	log *logger.Logger

	executor *Executor

	jobs  map[string]*jobState
	jobMu sync.RWMutex

	timerCh chan string

	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	userMacros       map[string]string
	vnodeLookup      func(spec.JobSpec) VnodeInfo
	shard            string
	emitter          ResultEmitter
	registerPerfdata func(spec.JobSpec, output.PerfDatum)
	periods          *timeperiod.Set
	rand             *rand.Rand
	randMu           sync.Mutex

	counters struct {
		started  atomic.Uint64
		finished atomic.Uint64
		skipped  atomic.Uint64
	}
}

type jobState struct {
	runtime         JobRuntime
	period          *timeperiod.Period
	nextAnniversary time.Time
	nextRun         time.Time
	timer           *time.Timer
	attempt         int
	running         bool
	lastDuration    time.Duration
	lastCPU         time.Duration
	lastRSS         int64
	lastDiskRead    int64
	lastDiskWrite   int64
	cpuMeasured     bool
	skipped         uint64
	state           string
	retrying        bool
	periodSkipped   bool
	identity        charts.JobIdentity
	softAttempts    int
	hardState       string
	softState       string
	checkInterval   time.Duration
	retryInterval   time.Duration
	maxAttempts     int
	timeoutState    string
	perfdata        map[string]output.PerfDatum
	statusLine      string
	longOutput      string
	jitterRange     time.Duration
}

// NewScheduler initialises the scheduler structures but does not start timers/workers.
func NewScheduler(cfg SchedulerConfig) (*Scheduler, error) {
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf("scheduler requires at least one job")
	}
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("scheduler workers must be > 0")
	}
	if cfg.Shard == "" {
		cfg.Shard = "default"
	}
	if cfg.Emitter == nil {
		cfg.Emitter = NewNoopEmitter()
	}

	userMacros := make(map[string]string, len(cfg.UserMacros))
	for k, v := range cfg.UserMacros {
		userMacros[strings.ToUpper(k)] = v
	}

	lookup := cfg.VnodeLookup
	if lookup == nil {
		lookup = func(sp spec.JobSpec) VnodeInfo {
			return VnodeInfo{Hostname: sp.Vnode}
		}
	}

	s := &Scheduler{
		log:              cfg.Logger,
		timerCh:          make(chan string, len(cfg.Jobs)),
		jobs:             make(map[string]*jobState, len(cfg.Jobs)),
		userMacros:       userMacros,
		vnodeLookup:      lookup,
		shard:            cfg.Shard,
		emitter:          cfg.Emitter,
		registerPerfdata: cfg.RegisterPerfdata,
		periods:          cfg.Periods,
		rand:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	workFn := func(ctx context.Context, job JobRuntime) ExecutionResult {
		return s.runJob(ctx, job)
	}

	exec, err := NewExecutor(ExecutorConfig{
		Logger:        cfg.Logger,
		Workers:       cfg.Workers,
		QueueCapacity: len(cfg.Jobs),
		Work:          workFn,
	})
	if err != nil {
		return nil, err
	}
	s.executor = exec

	now := time.Now()
	for idx, sp := range cfg.Jobs {
		jr := JobRuntime{
			ID:    buildJobID(sp, idx),
			Spec:  sp,
			Vnode: lookup(sp),
		}
		var period *timeperiod.Period
		if cfg.Periods != nil {
			var err error
			period, err = cfg.Periods.Resolve(sp.CheckPeriod)
			if err != nil {
				return nil, err
			}
		}
		baseline := now.Add(time.Duration(idx%len(cfg.Jobs)) * time.Second)
		identity := charts.NewJobIdentity(cfg.Shard, sp)
		js := &jobState{
			runtime:         jr,
			period:          period,
			nextAnniversary: baseline,
			nextRun:         baseline,
			state:           "UNKNOWN",
			softState:       "UNKNOWN",
			hardState:       "UNKNOWN",
			identity:        identity,
			checkInterval:   intervalOrDefault(sp.CheckInterval),
			retryInterval:   intervalOrDefault(sp.RetryInterval),
			maxAttempts:     maxInt(sp.MaxCheckAttempts, 1),
			timeoutState:    normalizeState(sp.TimeoutState),
			jitterRange:     sp.InterCheckJitter,
		}
		if js.retryInterval <= 0 {
			js.retryInterval = js.checkInterval
		}
		js.nextRun = s.applyJitter(js.nextAnniversary, js.jitterRange)
		s.jobs[jr.ID] = js
	}

	return s, nil
}

// Start launches timers, workers, and the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) error {
	if s.ctx != nil {
		return fmt.Errorf("scheduler already started")
	}
	s.ctx, s.cancel = context.WithCancel(ctx)

	for _, js := range s.jobs {
		s.armTimer(js)
	}

	s.executor.Start(s.ctx)

	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop cancels timers and waits for background goroutines to exit.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	for _, js := range s.jobs {
		if js.timer != nil {
			js.timer.Stop()
		}
	}
	s.executor.Stop()
	s.wg.Wait()
	s.ctx = nil
	s.cancel = nil
	if s.emitter != nil {
		_ = s.emitter.Close()
	}
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case jobID := <-s.timerCh:
			s.handleTimer(jobID)
		case res, ok := <-s.executor.Results():
			if !ok {
				return
			}
			s.handleResult(res)
		}
	}
}

func (s *Scheduler) handleTimer(jobID string) {
	s.jobMu.Lock()
	js, ok := s.jobs[jobID]
	if !ok {
		s.jobMu.Unlock()
		return
	}
	now := time.Now()
	if js.period != nil && !js.period.Allows(now) {
		js.periodSkipped = true
		nextAllowed := js.period.NextAllowed(now)
		if nextAllowed.IsZero() {
			nextAllowed = now.Add(js.checkInterval)
		}
		js.nextAnniversary = nextAllowed
		js.nextRun = s.applyJitter(nextAllowed, js.jitterRange)
		s.jobMu.Unlock()
		s.armTimer(js)
		return
	}
	js.periodSkipped = false
	js.nextAnniversary = s.advanceAnniversary(js.nextAnniversary, js.checkInterval, now)
	js.nextRun = s.applyJitter(js.nextAnniversary, js.jitterRange)
	s.jobMu.Unlock()

	s.armTimer(js)

	if queued, err := s.executor.Enqueue(js.runtime); err != nil {
		if s.log != nil {
			s.log.Errorf("nagios executor enqueue failed: %v", err)
		}
	} else if queued {
		s.counters.started.Add(1)
	} else {
		s.jobMu.Lock()
		js.skipped++
		s.jobMu.Unlock()
		s.counters.skipped.Add(1)
	}
}

func (s *Scheduler) handleResult(res ExecutionResult) {
	parsed := output.Parse(res.Output)
	if s.log != nil {
		s.log.Debugf("nagios: parsed perfdata entries=%d for job=%s", len(parsed.Perfdata), res.Job.Spec.Name)
	}
	res.Parsed = parsed

	s.jobMu.Lock()
	js, ok := s.jobs[res.Job.ID]
	var jobSpec spec.JobSpec
	var snapshot JobSnapshot
	var scheduleRetry bool
	var measured bool
	if ok {
		jobSpec = js.runtime.Spec
		js.running = false
		js.lastDuration = res.Duration
		js.lastCPU = res.Usage.User + res.Usage.System
		measured = res.Usage.User != 0 || res.Usage.System != 0 || res.Duration == 0
		js.cpuMeasured = measured
		js.lastRSS = res.Usage.MaxRSSBytes
		js.lastDiskRead = res.Usage.ReadBytes
		js.lastDiskWrite = res.Usage.WriteBytes
		js.statusLine = parsed.StatusLine
		js.longOutput = parsed.LongOutput
		js.updatePerfdata(parsed.Perfdata)
		prevHard := js.hardState
		js.recordResult(res.State)
		js.periodSkipped = false
		if js.retrying {
			base := time.Now()
			js.nextAnniversary = s.advanceAnniversary(base, js.retryInterval, base)
			js.nextRun = s.applyJitter(js.nextAnniversary, js.jitterRange)
			scheduleRetry = true
		}
		snapshot = JobSnapshot{
			HardState:     js.hardState,
			SoftState:     js.softState,
			PrevHardState: prevHard,
			Attempts:      js.softAttempts,
			Duration:      res.Duration,
			Timestamp:     res.End,
			Output:        parsed,
		}
	}
	s.jobMu.Unlock()

	if ok {
		s.counters.finished.Add(1)
		if scheduleRetry {
			s.armTimer(js)
		}
		if !measured && res.Duration > 0 && s.log != nil {
			s.log.Debugf("nagios job %s completed without CPU usage data; emitting zero", res.Job.Spec.Name)
		}
		s.registerPerfdataCharts(jobSpec, parsed.Perfdata)
		s.emitter.Emit(res.Job, res, snapshot)
	}
}

func (s *Scheduler) armTimer(js *jobState) {
	if s.ctx == nil {
		return
	}
	delay := time.Until(js.nextRun)
	if delay < 0 {
		delay = 0
	}
	jobID := js.runtime.ID

	if js.timer == nil {
		js.timer = time.AfterFunc(delay, func() {
			select {
			case s.timerCh <- jobID:
			case <-s.ctx.Done():
			}
		})
	} else {
		js.timer.Reset(delay)
	}
}

func (s *Scheduler) runJob(ctx context.Context, job JobRuntime) ExecutionResult {
	res := ExecutionResult{Job: job, Start: time.Now()}
	s.markRunning(job.ID, true)
	defer s.markRunning(job.ID, false)

	timeout := job.Spec.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}

	macroCtx := s.buildMacroContext(job)
	macroSet := BuildMacroSet(macroCtx)
	args := macroSet.CommandArgs
	if len(args) == 0 {
		args = job.Spec.Args
	}
	env := s.buildEnv(job.Spec.Environment, macroSet.Env)
	opts := ndexec.RunOptions{Env: env}
	if dir := job.Spec.WorkingDirectory; dir != "" {
		opts.Dir = dir
	}

	output, cmdStr, usage, err := ndexec.RunUnprivilegedWithOptionsUsage(s.log, timeout, opts, job.Spec.Plugin, args...)
	if s.log != nil {
		s.log.Debugf("nagios: raw plugin output job=%s output=%q", job.Spec.Name, string(output))
	}
	res.Output = output
	res.Err = err
	res.Command = cmdStr
	res.ExitCode = exitCodeFromError(err)
	res.End = time.Now()
	res.Duration = res.End.Sub(res.Start)
	res.State = s.stateFromResult(job, res.ExitCode, err)
	res.Usage = usage

	return res
}

func (s *Scheduler) buildMacroContext(job JobRuntime) MacroContext {
	state := StateInfo{
		ServiceState:       s.currentState(job.ID),
		ServiceAttempt:     s.currentAttempt(job.ID),
		ServiceMaxAttempts: maxInt(job.Spec.MaxCheckAttempts, 1),
		HostState:          "UP",
		HostStateID:        "0",
	}
	return MacroContext{
		Job:        job.Spec,
		UserMacros: s.userMacros,
		Vnode:      job.Vnode,
		State:      state,
	}
}

func (s *Scheduler) markRunning(jobID string, running bool) {
	s.jobMu.Lock()
	if js, ok := s.jobs[jobID]; ok {
		js.running = running
	}
	s.jobMu.Unlock()
}

func buildJobID(sp spec.JobSpec, idx int) string {
	vnode := sp.Vnode
	if vnode == "" {
		vnode = "local"
	}
	return fmt.Sprintf("%s@%s#%d", sp.Name, vnode, idx)
}

func (s *Scheduler) CollectMetrics() map[string]int64 {
	metrics := make(map[string]int64)
	stats := s.executor.Stats()
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerJobs, "running")] = int64(stats.Executing)
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerJobs, "queued")] = int64(stats.QueueDepth)
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerJobs, "scheduled")] = int64(s.scheduledCount())
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerRate, "started")] = int64(s.counters.started.Load())
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerRate, "finished")] = int64(s.counters.finished.Load())
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerRate, "skipped")] = int64(s.counters.skipped.Load())
	metrics[charts.SchedulerMetricKey(s.shard, charts.ChartSchedulerNext, "next")] = s.nextRunDelay().Nanoseconds()

	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	for _, js := range s.jobs {
		id := js.identity
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "ok")] = boolToInt(strings.EqualFold(js.state, "OK"))
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "warning")] = boolToInt(strings.EqualFold(js.state, "WARNING"))
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "critical")] = boolToInt(strings.EqualFold(js.state, "CRITICAL"))
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "unknown")] = boolToInt(strings.EqualFold(js.state, "UNKNOWN"))
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "attempt")] = int64(js.currentAttempt())
		metrics[id.TelemetryMetricID(charts.TelemetryStateMetric, "max_attempts")] = int64(js.maxAttempts)
		metrics[id.TelemetryMetricID(charts.TelemetryRuntimeMetric, "running")] = boolToInt(js.running)
		metrics[id.TelemetryMetricID(charts.TelemetryRuntimeMetric, "retrying")] = boolToInt(js.retrying)
		missingCPU := !js.cpuMeasured && js.lastDuration > 0
		metrics[id.TelemetryMetricID(charts.TelemetryRuntimeMetric, "skipped")] = boolToInt(js.periodSkipped)
		metrics[id.TelemetryMetricID(charts.TelemetryRuntimeMetric, "cpu_missing")] = boolToInt(missingCPU)
		metrics[id.TelemetryMetricID(charts.TelemetryLatencyMetric, "duration")] = js.lastDuration.Nanoseconds()
		cpuNs := js.lastCPU.Nanoseconds()
		metrics[id.TelemetryMetricID(charts.TelemetryCPUMetric, "cpu_time")] = cpuNs
		metrics[id.TelemetryMetricID(charts.TelemetryMemoryMetric, "rss")] = js.lastRSS
		metrics[id.TelemetryMetricID(charts.TelemetryDiskMetric, "read")] = js.lastDiskRead
		metrics[id.TelemetryMetricID(charts.TelemetryDiskMetric, "write")] = js.lastDiskWrite

		if len(js.perfdata) > 0 {
			for labelID, datum := range js.perfdata {
				scale := units.NewScale(datum.Unit)
				metrics[id.PerfdataMetricID(labelID, "value")] = scale.Apply(datum.Value)
				if datum.Min != nil {
					metrics[id.PerfdataMetricID(labelID, "min")] = scale.Apply(*datum.Min)
				}
				if datum.Max != nil {
					metrics[id.PerfdataMetricID(labelID, "max")] = scale.Apply(*datum.Max)
				}
				s.setRangeMetrics(metrics, id, labelID, "warn", datum.Warn, scale)
				s.setRangeMetrics(metrics, id, labelID, "crit", datum.Crit, scale)
			}
		}
	}

	return metrics
}

func (s *Scheduler) nextRunDelay() time.Duration {
	min := 24 * time.Hour
	if len(s.jobs) == 0 {
		return 0
	}
	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	for _, js := range s.jobs {
		delta := time.Until(js.nextRun)
		if delta < 0 {
			delta = 0
		}
		if delta < min {
			min = delta
		}
	}
	return min
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func (s *Scheduler) scheduledCount() int {
	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	return len(s.jobs)
}

func (s *Scheduler) advanceAnniversary(current time.Time, interval time.Duration, now time.Time) time.Time {
	interval = intervalOrDefault(interval)
	if current.IsZero() {
		current = now
	}
	next := current.Add(interval)
	if !next.After(now) {
		diff := now.Sub(next)
		steps := diff/interval + 1
		next = next.Add(time.Duration(steps) * interval)
	}
	return next
}

func (s *Scheduler) applyJitter(base time.Time, jitter time.Duration) time.Time {
	if jitter <= 0 {
		return base
	}
	if s.rand == nil {
		return base
	}
	s.randMu.Lock()
	val := s.rand.Float64()
	s.randMu.Unlock()
	if val <= 0 {
		return base
	}
	return base.Add(time.Duration(val * float64(jitter)))
}

func (s *Scheduler) setRangeMetrics(metrics map[string]int64, id charts.JobIdentity, labelID, kind string, rng *output.ThresholdRange, scale units.Scale) {
	definedKey := id.PerfdataMetricID(labelID, kind+"_defined")
	inclusiveKey := id.PerfdataMetricID(labelID, kind+"_inclusive")
	lowKey := id.PerfdataMetricID(labelID, kind+"_low")
	highKey := id.PerfdataMetricID(labelID, kind+"_high")
	lowDefinedKey := id.PerfdataMetricID(labelID, kind+"_low_defined")
	highDefinedKey := id.PerfdataMetricID(labelID, kind+"_high_defined")
	if rng == nil {
		metrics[definedKey] = 0
		metrics[inclusiveKey] = 0
		metrics[lowKey] = 0
		metrics[highKey] = 0
		metrics[lowDefinedKey] = 0
		metrics[highDefinedKey] = 0
		return
	}
	metrics[definedKey] = 1
	metrics[inclusiveKey] = boolToInt(rng.Inclusive)
	if v, ok := rangeBoundMetric(scale, rng.Low); ok {
		metrics[lowKey] = v
		metrics[lowDefinedKey] = 1
	} else {
		metrics[lowKey] = 0
		metrics[lowDefinedKey] = 0
	}
	if v, ok := rangeBoundMetric(scale, rng.High); ok {
		metrics[highKey] = v
		metrics[highDefinedKey] = 1
	} else {
		metrics[highKey] = 0
		metrics[highDefinedKey] = 0
	}
}

func rangeBoundMetric(scale units.Scale, val *float64) (int64, bool) {
	if val == nil {
		return 0, false
	}
	if math.IsNaN(*val) || math.IsInf(*val, 0) {
		return 0, false
	}
	return scale.Apply(*val), true
}

func (s *Scheduler) registerPerfdataCharts(job spec.JobSpec, perf []output.PerfDatum) {
	if s.registerPerfdata == nil {
		return
	}
	for _, datum := range perf {
		label := strings.TrimSpace(datum.Label)
		if label == "" {
			continue
		}
		s.registerPerfdata(job, datum)
	}
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (s *Scheduler) stateFromResult(job JobRuntime, exitCode int, err error) string {
	if err == nil {
		return "OK"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return normalizeState(job.Spec.TimeoutState)
	}
	switch exitCode {
	case 0:
		return "OK"
	case 1:
		return "WARNING"
	case 2:
		return "CRITICAL"
	case 3:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

func intervalOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Minute
	}
	return d
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeState(state string) string {
	s := strings.ToUpper(state)
	switch s {
	case "OK", "WARNING", "CRITICAL", "UNKNOWN":
		return s
	default:
		return "UNKNOWN"
	}
}

func (s *Scheduler) currentState(jobID string) string {
	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	if js, ok := s.jobs[jobID]; ok && js.state != "" {
		return js.state
	}
	return "UNKNOWN"
}

func (s *Scheduler) currentAttempt(jobID string) int {
	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	if js, ok := s.jobs[jobID]; ok {
		return js.currentAttempt()
	}
	return 1
}

func (s *Scheduler) buildEnv(jobEnv map[string]string, macroEnv map[string]string) []string {
	merged := make(map[string]string)
	for _, kv := range os.Environ() {
		if eq := strings.Index(kv, "="); eq > 0 {
			merged[kv[:eq]] = kv[eq+1:]
		}
	}
	for k, v := range jobEnv {
		merged[k] = replaceMacro(v, macroEnv)
	}
	for k, v := range macroEnv {
		merged[k] = v
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, k := range keys {
		result = append(result, fmt.Sprintf("%s=%s", k, merged[k]))
	}
	return result
}

func (js *jobState) recordResult(state string) {
	state = normalizeState(state)
	js.softState = state
	if state == "OK" {
		js.softAttempts = 0
		js.hardState = "OK"
		js.retrying = false
	} else {
		js.softAttempts++
		if js.softAttempts >= js.maxAttempts {
			js.hardState = state
			js.retrying = false
		} else {
			js.retrying = true
		}
	}
	js.state = state
}

func (js *jobState) updatePerfdata(perf []output.PerfDatum) {
	if len(perf) == 0 {
		js.perfdata = nil
		return
	}
	mp := make(map[string]output.PerfDatum, len(perf))
	for _, datum := range perf {
		labelID := ids.Sanitize(datum.Label)
		if labelID == "" {
			continue
		}
		mp[labelID] = datum
	}
	js.perfdata = mp
}

func (js *jobState) currentAttempt() int {
	if js == nil {
		return 1
	}
	if strings.EqualFold(js.state, "OK") || js.state == "" {
		return 1
	}
	attempt := js.softAttempts
	if attempt <= 0 {
		attempt = 1
	}
	if js.retrying {
		attempt++
	}
	if attempt > js.maxAttempts {
		return js.maxAttempts
	}
	return attempt
}
