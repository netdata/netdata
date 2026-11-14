// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner owns admission and supervised execution of prepared DEM runtimes.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// frameMaxBytes is the runner protocol's 4 MiB ceiling on the request and on
// every record, shared with the JS runner (run.mjs framing limit).
const frameMaxBytes = 4 << 20

// eventLimit is the per-run diagnostic event retention documented in the
// plugin and runtime READMEs. Later events are counted as dropped; every
// retained event is stored in the run's history record.
const eventLimit = 500

// textLimit is the character budget of each diagnostic text, shared with the
// JS runner's clipping (protocol.cjs) and documented in its README.
const textLimit = 2000

const cleanupTimeout = 10 * time.Second

// historyWriter borrows the process journal; uncertain appends must not be replayed.
type historyWriter interface {
	AppendRun(context.Context, synthetic.HistoryPhase, synthetic.Run) (bool, error)
	Sync(context.Context) error
}

// artifactStore creates each attempt's run work directory and finalizes it:
// publishing its captures and removing the work.
type artifactStore interface {
	Begin(runID, jobID, vnodeGUID string) (string, error)
	Finalize(context.Context, string, []synthetic.Capture) ([]synthetic.Artifact, error)
}

type Engine struct {
	*executionOwner
	source        Source
	history       historyWriter
	unrecorded    bool // rehearsal and qualification views record no history
	artifacts     artifactStore
	start         startFunc
	platform      func() error
	checks        *checkOwner
	qualification bool
}

// New returns the monitoring engine. Every monitored attempt records history
// and owns a run work directory, so both owners are required.
func New(source Source, history historyWriter, artifacts artifactStore, concurrency int) (*Engine, error) {
	if concurrency <= 0 {
		return nil, errors.New("browser concurrency must be positive")
	}
	if history == nil || artifacts == nil {
		return nil, errors.New("runner requires history and artifact owners")
	}
	return newEngine(source, history, artifacts, concurrency), nil
}

func newEngine(source Source, history historyWriter, artifacts artifactStore, concurrency int) *Engine {
	return &Engine{
		executionOwner: newExecutionOwner(concurrency),
		checks:         &checkOwner{},
		source:         source,
		history:        history,
		artifacts:      artifacts,
		platform:       runtimeAllowed,
		start: func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
			return ndexec.StartUnprivilegedProcessTree(ctx, opts, bin, args...)
		},
	}
}

// unrecordedView shares this engine's admission and fatal owner but records no
// history and uses its own source, artifact store and check coalescing.
func (e *Engine) unrecordedView(source Source, artifacts artifactStore) *Engine {
	return &Engine{
		executionOwner: e.executionOwner,
		source:         source,
		unrecorded:     true,
		checks:         &checkOwner{},
		qualification:  e.qualification,
		artifacts:      artifacts,
		start:          e.start,
		platform:       e.platform,
	}
}

func (e *Engine) Fatal() <-chan error { return e.fatal }

func (e *Engine) poison(err error) {
	e.poisonOnce.Do(func() {
		e.mu.Lock()
		close(e.poisoned)
		e.mu.Unlock()
		e.fatal <- errors.Join(ndexec.ErrTreeNotDrained, err)
	})
}

// attempt is one execution's mutable record: the redacted run and the run
// work directory, which exists once created and must then be finalized.
type attempt struct {
	request synthetic.Request
	redact  *textRedactor
	run     synthetic.Run
	work    string
}

func newAttempt(request synthetic.Request) *attempt {
	redact := newRedactor(request.Secrets)
	var id [16]byte
	_, _ = rand.Read(id[:]) // never returns an error since Go 1.24
	a := &attempt{
		request: request,
		redact:  redact,
		run: synthetic.Run{
			ID:           hex.EncodeToString(id[:]),
			JobID:        request.JobID(),
			VnodeGUID:    request.VnodeGUID,
			Kind:         request.Kind,
			Name:         redact.text(request.Name),
			Target:       redact.text(redact.patterns.ApplyURL(request.URL)),
			Outcome:      synthetic.Error,
			CaptureState: synthetic.CaptureDisabled,
			Events:       []synthetic.Event{},
			Artifacts:    []synthetic.Artifact{},
		},
	}
	if request.Capture {
		a.run.CaptureState = synthetic.CaptureUnavailable
	}
	return a
}

// execute runs an immediate attempt without scheduling metadata, as
// rehearsal and qualification do.
func (e *Engine) execute(ctx context.Context, request synthetic.Request) synthetic.Execution {
	return e.ExecuteScheduled(ctx, request, nil, nil)
}

// ExecuteScheduled runs one attempt through admission. admitted, when set,
// snapshots the latest pending occurrence only after admission. Drained is
// false when the process tree may still be alive; the slot then stays held.
func (e *Engine) ExecuteScheduled(ctx context.Context, request synthetic.Request, state func(synthetic.JobState), admitted func(time.Time) synthetic.Schedule) synthetic.Execution {
	a := newAttempt(request)
	job, err := e.admit(ctx, request.JobID(), state)
	if err != nil {
		// Nothing ran: no history, no work directory.
		a.stop(err, ctx, ctx)
		a.run.Error = a.redact.text(err.Error())
		a.run.CompletedUS = time.Now().UnixMicro()
		return synthetic.Execution{Run: a.run, Drained: !errors.Is(err, ndexec.ErrTreeNotDrained)}
	}
	drained := e.runAdmitted(ctx, a, state, admitted)
	if drained {
		e.release(job)
	}
	return synthetic.Execution{Run: a.run, Drained: drained}
}

// admit waits for the job's turn at a browser slot. A slot granted after
// poisoning or after the caller stopped is released again.
func (e *Engine) admit(ctx context.Context, jobID string, state func(synthetic.JobState)) (*admissionJob, error) {
	if e.isPoisoned() {
		return nil, ndexec.ErrTreeNotDrained
	}
	notify(state, synthetic.JobWaiting)
	job, err := e.acquire(ctx, executionKey{jobID: jobID, qualification: e.qualification})
	if err != nil {
		return nil, err
	}
	if e.isPoisoned() {
		e.release(job)
		return nil, ndexec.ErrTreeNotDrained
	}
	if err := ctx.Err(); err != nil {
		e.release(job)
		return nil, err
	}
	return job, nil
}

// runAdmitted runs an admitted attempt to completion and reports whether its
// process tree is known drained. Every drained path records completion once.
func (e *Engine) runAdmitted(ctx context.Context, a *attempt, state func(synthetic.JobState), admitted func(time.Time) synthetic.Schedule) (drained bool) {
	admittedAt := time.Now()
	if admitted != nil {
		schedule := admitted(admittedAt)
		a.run.ScheduledUS, a.run.QueuedUS = schedule.DueUS, schedule.QueuedUS
	}
	a.run.StartedUS = admittedAt.UnixMicro()
	attemptCtx, cancel := context.WithTimeout(ctx, a.request.Timeout)
	defer cancel()

	if err := synthetic.ValidateRequest(a.request); err != nil {
		e.failBeforeStart(a, err)
		return true
	}
	if err := e.platform(); err != nil {
		e.failBeforeStart(a, err)
		return true
	}
	use, err := e.beginUse()
	if err != nil {
		e.failBeforeStart(a, err)
		return true
	}
	defer func() { _ = use.Finish(drained) }()
	paths := use.Paths()
	if err := checkFiles(paths); err != nil {
		e.failBeforeStart(a, err)
		return true
	}
	identity := paths.Identity
	a.run.Runtime = &identity
	e.save(attemptCtx, synthetic.HistoryStart, a)

	raw, err := e.prepareWork(attemptCtx, a, admittedAt)
	if err != nil {
		a.run.Error = a.redact.text(err.Error())
		e.complete(a, nil)
		return true
	}
	captures, drained := e.launch(ctx, attemptCtx, a, use, raw, state)
	if !drained {
		// Fatal was signalled before executeProcess joined the local pipe readers.
		// The command owner must fail-stop without closing stores or this work.
		return false
	}
	e.complete(a, captures)
	return true
}

// failBeforeStart records an admitted attempt that failed before its start
// record as an observed error. No work directory exists yet.
func (e *Engine) failBeforeStart(a *attempt, err error) {
	a.run.Error = a.redact.text(err.Error())
	cleanup, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	e.save(cleanup, synthetic.HistoryStart, a)
	e.complete(a, nil)
}

// prepareWork creates the run work directory and encodes the wire request.
func (e *Engine) prepareWork(attemptCtx context.Context, a *attempt, admittedAt time.Time) ([]byte, error) {
	work, err := e.artifacts.Begin(a.run.ID, a.run.JobID, a.run.VnodeGUID)
	if err != nil {
		return nil, fmt.Errorf("create run work directory: %w", err)
	}
	a.work = work
	r := a.request
	req := wireRequest{
		Extensions: requestedFeatures(r),
		Version:    synthetic.ExecutionContract,
		Kind:       r.Kind,
		RunID:      a.run.ID,
		WorkDir:    work,
		Script:     r.Script,
		ScriptPath: r.ScriptPath,
		URL:        r.URL,
		TimeoutMS:  r.Timeout.Milliseconds(),
		Capture:    r.Capture,
		Secrets:    r.Secrets,
	}
	if r.Kind == synthetic.Journey {
		deadline, _ := attemptCtx.Deadline()
		if req.StopAtMonotonicMS, err = journeyStopAt(deadline, admittedAt); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(req)
	if err == nil && len(raw) > frameMaxBytes {
		err = errors.New("runner request exceeds 4 MiB transport limit")
	}
	return raw, err
}

// launch runs the prepared request and records what the supervisor and the
// reporter observed. It returns the captures eligible for publication and
// whether the process tree is known drained.
func (e *Engine) launch(ctx, attemptCtx context.Context, a *attempt, use Use, raw []byte, state func(synthetic.JobState)) ([]synthetic.Capture, bool) {
	sink := newSink(a.request, a.redact)
	started := time.Now()
	notify(state, synthetic.JobRunning)
	paths := use.Paths()
	tree, err := e.executeProcess(attemptCtx, paths, use.Completion(), []string{paths.EntryPoint}, raw, sink.consume, sink.stderr)
	if err != nil {
		// Nothing started, so nothing was reported.
		a.stop(attemptCtx.Err(), ctx, attemptCtx)
		a.run.Error = a.redact.text(err.Error())
		return nil, true
	}
	elapsed := float64(time.Since(started)) / float64(time.Millisecond)
	a.run.DurationMS = &elapsed
	snapshot := sink.snapshot()
	a.observe(snapshot)
	a.classify(ctx, attemptCtx, tree, snapshot)
	if !tree.Drained {
		return nil, false
	}
	captures, ineligible := eligibleCaptures(&a.run, snapshot)
	joinCaptures(&a.run, captures, ineligible)
	return captures, true
}

// observe copies the reporter's observations into the run. Observed test and
// measurement facts survive a later shutdown deadline.
func (a *attempt) observe(s sinkSnapshot) {
	run := &a.run
	run.FailureEvidence = s.evidence
	run.BrowserSummary = s.browser
	run.Captures = s.captures
	run.Events = s.events
	run.DroppedEvents = s.dropped
	run.Tests = s.partial
	if s.err == nil && s.result != nil {
		if s.result.Tests != nil {
			run.Tests = s.result.Tests
		}
		run.Metrics = s.result.Metrics
	}
}

// classify decides the outcome. Supervisor facts outrank the reporter: an
// unverified tree, a deadline or a cancellation wins over a reported result,
// but a reported first failure keeps its text.
func (a *attempt) classify(ctx, attemptCtx context.Context, tree ndexec.TreeResult, s sinkSnapshot) {
	run := &a.run
	if !tree.Drained {
		run.Error = a.redact.text("process-tree completion is unverified: " + errorText(tree.Err))
		return
	}
	reported := s.err == nil && s.result != nil
	switch {
	case a.stop(attemptCtx.Err(), ctx, attemptCtx):
		run.Error = stopText(run.TerminationReason)
	case s.err != nil:
		run.Error = a.redact.text(s.err.Error())
	case s.result == nil:
		run.Error = a.redact.text("runner exited without a terminal result: " + errorText(tree.Err))
	case s.result.Status == synthetic.Success && tree.Err != nil:
		run.Error = a.redact.text("runner exited unsuccessfully after reporting success")
	default:
		run.Outcome = s.result.Status
		run.TerminationReason = s.result.TerminationReason
		run.Error = a.redact.text(s.result.Error)
		run.CaptureState = s.result.CaptureState
	}
	if reported && run.TerminationReason != "" && s.result.Error != "" {
		run.Error = a.redact.text(s.result.Error)
	}
}

// stop records a caller cancellation or an expired deadline and reports
// whether err was one. Before admission attemptCtx is the caller's context and
// no workflow ran, so an expired deadline is a cancellation, not a timeout.
func (a *attempt) stop(err error, ctx, attemptCtx context.Context) bool {
	switch {
	case errors.Is(err, context.Canceled):
		a.run.Outcome, a.run.TerminationReason = synthetic.Cancelled, synthetic.CallerCancelled
	case errors.Is(err, context.DeadlineExceeded):
		a.run.Outcome, a.run.TerminationReason = synthetic.Timeout, deadlineReason(ctx, attemptCtx)
		if a.run.StartedUS == 0 {
			a.run.Outcome = synthetic.Cancelled
		}
	default:
		return false
	}
	return true
}

func stopText(reason synthetic.TerminationReason) string {
	switch reason {
	case synthetic.CallerCancelled:
		return "execution cancelled"
	case synthetic.CallerDeadline:
		return "caller deadline exceeded"
	default:
		return "overall execution deadline exceeded"
	}
}

// eligibleCaptures splits reported candidates into those to publish and those
// to explain: interrupted captures publish only after a journey budget stop.
func eligibleCaptures(run *synthetic.Run, s sinkSnapshot) (captures, ineligible []synthetic.Capture) {
	if s.err != nil || s.result == nil {
		return nil, nil
	}
	captures = append(captures, s.result.Artifacts...)
	if run.TerminationReason == synthetic.ExecutionBudget {
		return append(captures, s.result.InterruptedArtifacts...), nil
	}
	return captures, s.result.InterruptedArtifacts
}

// complete finalizes the run work directory, when one exists, and records
// completion. It owns a fixed cleanup budget independent of cancelled workflow input.
func (e *Engine) complete(a *attempt, captures []synthetic.Capture) {
	cleanup, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if a.work != "" {
		e.publish(cleanup, a, captures)
	}
	a.run.CompletedUS = time.Now().UnixMicro()
	e.save(cleanup, synthetic.HistoryComplete, a)
}

func (e *Engine) publish(ctx context.Context, a *attempt, captures []synthetic.Capture) {
	run := &a.run
	artifacts, err := e.artifacts.Finalize(ctx, run.ID, captures)
	if err != nil {
		capturePublicationFailed(run, captures, err)
		run.Error = joinText(run.Error, a.redact.text("artifact publication: "+err.Error()))
		if run.CaptureState != synthetic.CaptureDisabled {
			run.CaptureState = synthetic.CaptureUnavailable
		}
		return
	}
	run.Artifacts = artifacts
	capturePublicationSucceeded(run, captures)
	if run.CaptureState != synthetic.CaptureDisabled && len(captures) > 0 {
		run.CaptureState = synthetic.CaptureUnavailable
		if len(artifacts) > 0 {
			run.CaptureState = synthetic.CaptureAvailable
		}
	}
}

func (e *Engine) save(ctx context.Context, phase synthetic.HistoryPhase, a *attempt) {
	if e.unrecorded {
		return
	}
	run := &a.run
	// A value snapshot is immutable; later completion must not alter start data.
	attempted, err := e.history.AppendRun(ctx, phase, synthetic.CloneRun(*run))
	if err != nil {
		qualifier := "not appended"
		if attempted {
			qualifier = "append outcome uncertain"
		}
		run.HistoryError = joinText(run.HistoryError, a.redact.text(string(phase)+" history "+qualifier+": "+err.Error()))
	}
	if err = e.history.Sync(ctx); err != nil {
		run.HistoryError = joinText(run.HistoryError, a.redact.text(string(phase)+" history sync: "+err.Error()))
	}
}

func notify(state func(synthetic.JobState), value synthetic.JobState) {
	if state != nil {
		state(value)
	}
}

func errorText(err error) string {
	if err == nil {
		return "no result"
	}
	return err.Error()
}

func joinText(first, second string) string {
	if first == "" {
		return clip(second)
	}
	return clip(first + "; " + second)
}

// journeyStopAt reserves reporting time inside the effective admitted budget.
// Sample the shared clock first so sampling latency cannot extend the deadline.
func journeyStopAt(deadline, admitted time.Time) (int64, error) {
	now, err := monotonicNow()
	if err != nil {
		return 0, err
	}
	reserve := min(5*time.Second, max(0, deadline.Sub(admitted))/5)
	return now.Milliseconds() + (time.Until(deadline) - reserve).Milliseconds(), nil
}

func deadlineReason(caller, attempt context.Context) synthetic.TerminationReason {
	callerDeadline, ok := caller.Deadline()
	attemptDeadline, _ := attempt.Deadline()
	if ok && !callerDeadline.After(attemptDeadline) {
		return synthetic.CallerDeadline
	}
	return synthetic.ExecutionDeadline
}
