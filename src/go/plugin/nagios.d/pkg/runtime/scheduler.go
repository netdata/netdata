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
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/timeperiod"
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
	RegisterPerfdata func(spec.JobSpec, string)
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
	registerPerfdata func(spec.JobSpec, string)
	periods          *timeperiod.Set
	rand             *rand.Rand
	randMu           sync.Mutex
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
	skipped         uint64
	state           string
	retrying        bool
	periodSkipped   bool
	chartJobName    string
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
		js := &jobState{
			runtime:         jr,
			period:          period,
			nextAnniversary: baseline,
			nextRun:         baseline,
			state:           "UNKNOWN",
			softState:       "UNKNOWN",
			hardState:       "UNKNOWN",
			chartJobName:    ids.Sanitize(sp.Name),
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
	} else if !queued {
		s.jobMu.Lock()
		js.skipped++
		s.jobMu.Unlock()
	}
}

func (s *Scheduler) handleResult(res ExecutionResult) {
	parsed := output.Parse(res.Output)
	res.Parsed = parsed

	s.jobMu.Lock()
	js, ok := s.jobs[res.Job.ID]
	var jobSpec spec.JobSpec
	var snapshot JobSnapshot
	var scheduleRetry bool
	if ok {
		jobSpec = js.runtime.Spec
		js.running = false
		js.lastDuration = res.Duration
		js.lastCPU = res.Usage.User + res.Usage.System
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
		if scheduleRetry {
			s.armTimer(js)
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

	macroCtx := MacroContext{
		Job:        job.Spec,
		UserMacros: s.userMacros,
		Vnode:      job.Vnode,
		State:      StateInfo{ServiceState: s.currentState(job.ID)},
	}
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
	metrics[charts.MetricKey(s.shard, "scheduler", "scheduler_queue", "queue_depth")] = int64(stats.QueueDepth)
	metrics[charts.MetricKey(s.shard, "scheduler", "scheduler_queue", "waiting")] = int64(stats.Waiting)
	metrics[charts.MetricKey(s.shard, "scheduler", "scheduler_queue", "executing")] = int64(stats.Executing)
	metrics[charts.MetricKey(s.shard, "scheduler", "scheduler_skipped", "skipped")] = int64(stats.SkippedEnqueue)
	metrics[charts.MetricKey(s.shard, "scheduler", "scheduler_next", "next_ms")] = s.nextRunMs()

	s.jobMu.RLock()
	defer s.jobMu.RUnlock()
	for _, js := range s.jobs {
		jobName := js.chartJobName
		metrics[charts.MetricKey(s.shard, jobName, "state", "ok")] = boolToInt(strings.EqualFold(js.state, "OK"))
		metrics[charts.MetricKey(s.shard, jobName, "state", "warning")] = boolToInt(strings.EqualFold(js.state, "WARNING"))
		metrics[charts.MetricKey(s.shard, jobName, "state", "critical")] = boolToInt(strings.EqualFold(js.state, "CRITICAL"))
		metrics[charts.MetricKey(s.shard, jobName, "state", "unknown")] = boolToInt(strings.EqualFold(js.state, "UNKNOWN"))
		metrics[charts.MetricKey(s.shard, jobName, "runtime", "running")] = boolToInt(js.running)
		metrics[charts.MetricKey(s.shard, jobName, "runtime", "retrying")] = boolToInt(js.retrying)
		metrics[charts.MetricKey(s.shard, jobName, "runtime", "skipped")] = boolToInt(js.periodSkipped)
		metrics[charts.MetricKey(s.shard, jobName, "latency", "duration")] = int64(js.lastDuration / time.Millisecond)
		cpuMs := int64(js.lastCPU / time.Millisecond)
		if cpuMs == 0 {
			cpuMs = int64(js.lastDuration / time.Millisecond)
		}
		metrics[charts.MetricKey(s.shard, jobName, "cpu", "cpu_time")] = cpuMs
		metrics[charts.MetricKey(s.shard, jobName, "mem", "rss")] = js.lastRSS
		metrics[charts.MetricKey(s.shard, jobName, "disk", "read")] = js.lastDiskRead
		metrics[charts.MetricKey(s.shard, jobName, "disk", "write")] = js.lastDiskWrite

		if len(js.perfdata) > 0 {
			for labelID, datum := range js.perfdata {
				suffix := fmt.Sprintf("perf.%s", labelID)
				metrics[charts.MetricKey(s.shard, jobName, suffix, "value")] = metricValue(datum.Value)
				if datum.Min != nil {
					metrics[charts.MetricKey(s.shard, jobName, suffix, "min")] = metricValue(*datum.Min)
				}
				if datum.Max != nil {
					metrics[charts.MetricKey(s.shard, jobName, suffix, "max")] = metricValue(*datum.Max)
				}
				s.setRangeMetrics(metrics, jobName, suffix, "warn", datum.Warn)
				s.setRangeMetrics(metrics, jobName, suffix, "crit", datum.Crit)
			}
		}
	}

	return metrics
}

func (s *Scheduler) nextRunMs() int64 {
	min := time.Hour * 24
	if len(s.jobs) == 0 {
		return 0
	}
	for _, js := range s.jobs {
		delta := time.Until(js.nextRun)
		if delta < 0 {
			delta = 0
		}
		if delta < min {
			min = delta
		}
	}
	return int64(min / time.Millisecond)
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func metricValue(v float64) int64 {
	return int64(math.Round(v))
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

func (s *Scheduler) setRangeMetrics(metrics map[string]int64, jobName, suffix, kind string, rng *output.ThresholdRange) {
	definedKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_defined")
	inclusiveKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_inclusive")
	lowKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_low")
	highKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_high")
	lowDefinedKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_low_defined")
	highDefinedKey := charts.MetricKey(s.shard, jobName, suffix, kind+"_high_defined")
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
	if v, ok := rangeBoundMetric(rng.Low); ok {
		metrics[lowKey] = v
		metrics[lowDefinedKey] = 1
	} else {
		metrics[lowKey] = 0
		metrics[lowDefinedKey] = 0
	}
	if v, ok := rangeBoundMetric(rng.High); ok {
		metrics[highKey] = v
		metrics[highDefinedKey] = 1
	} else {
		metrics[highKey] = 0
		metrics[highDefinedKey] = 0
	}
}

func rangeBoundMetric(val *float64) (int64, bool) {
	if val == nil {
		return 0, false
	}
	if math.IsNaN(*val) || math.IsInf(*val, 0) {
		return 0, false
	}
	return metricValue(*val), true
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
		s.registerPerfdata(job, label)
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
	js.state = js.hardState
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
