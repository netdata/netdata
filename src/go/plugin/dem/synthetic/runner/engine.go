// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner owns admission and supervised execution of prepared DEM runtimes.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// FrameMaxBytes is the existing DEM transport's 4 MiB request/record ceiling.
// Event retention is separate: a verbose workflow does not retain every frame.
const FrameMaxBytes = 4 << 20

const EventLimit = 500

const TextLimit = 2000

const cleanupTimeout = 10 * time.Second

// historyWriter borrows the process journal; uncertain appends must not be replayed.
type historyWriter interface {
	AppendRun(context.Context, string, synthetic.Run) (bool, error)
	Sync(context.Context) error
}

type ArtifactStore interface {
	Begin(string, string, string) (string, error)
	Finalize(context.Context, string, []synthetic.Capture) ([]synthetic.Artifact, error)
}

type Engine struct {
	*executionOwner
	source        Source
	browserSource *browserSource
	history       historyWriter
	unrecorded    bool
	artifacts     ArtifactStore
	start         startFunc
	platform      func() error
	checks        *checkOwner
}

func New(source Source, history historyWriter, artifacts ArtifactStore, concurrency int) (*Engine, error) {
	if concurrency <= 0 {
		return nil, errors.New("browser concurrency must be positive")
	}
	return &Engine{
		executionOwner: &executionOwner{
			limit:    concurrency,
			jobs:     make(map[executionKey]*admissionJob),
			fatal:    make(chan error, 1),
			poisoned: make(chan struct{}),
			idleFor:  browserIdleTimeout,
			closing:  make(chan struct{}),
			closed:   make(chan struct{}),
		},
		checks:        &checkOwner{},
		source:        source,
		browserSource: &browserSource{},
		history:       history,
		artifacts:     artifacts,
		platform:      runtimeAllowed,
		start: func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
			return ndexec.StartUnprivilegedProcessTree(ctx, opts, bin, args...)
		},
	}, nil
}

func (e *Engine) unrecordedView(source Source, artifacts ArtifactStore) *Engine {
	return &Engine{
		executionOwner: e.executionOwner,
		source:         source,
		browserSource:  &browserSource{},
		unrecorded:     true,
		checks:         &checkOwner{},
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

func (e *Engine) Execute(ctx context.Context, request synthetic.Request, state func(string)) synthetic.Execution {
	return e.ExecuteScheduled(ctx, request, state, nil)
}

// ExecuteScheduled snapshots the latest pending occurrence only after admission.
// Immediate diagnostics and qualification use Execute without scheduling metadata.
func (e *Engine) ExecuteScheduled(ctx context.Context, request synthetic.Request, state func(string), admitted func(time.Time) synthetic.Schedule) synthetic.Execution {
	redact := newRedactor(request.Secrets)
	run := synthetic.Run{
		JobID:        request.JobID(),
		VnodeGUID:    request.VnodeGUID,
		Kind:         request.Kind,
		Name:         redact.text(request.Name),
		Target:       redact.text(redact.patterns.ApplyURL(request.URL)),
		Outcome:      synthetic.Error,
		CaptureState: "disabled",
		Events:       []synthetic.Event{},
		Artifacts:    []synthetic.Artifact{},
	}
	if request.Capture {
		run.CaptureState = "unavailable"
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		run.Error = redact.text(err.Error())
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	run.ID = hex.EncodeToString(random[:])
	finishUnstarted := func(err error, drained bool) synthetic.Execution {
		if errors.Is(err, context.Canceled) {
			run.Outcome = synthetic.Cancelled
		} else if errors.Is(err, context.DeadlineExceeded) {
			run.Outcome = synthetic.Cancelled
			if run.StartedUS > 0 {
				run.Outcome = synthetic.Timeout
			}
		}
		run.Error = redact.text(err.Error())
		run.CompletedUS = time.Now().UnixMicro()
		return synthetic.Execution{
			Run:     run,
			Drained: drained,
		}
	}
	if e.isPoisoned() {
		return finishUnstarted(ndexec.ErrTreeNotDrained, false)
	}
	if state != nil {
		state("waiting")
	}
	var scope *browserSource
	if request.Kind == synthetic.Journey && request.BrowserMode == "reuse" {
		scope = e.browserSource
	}
	job, err := e.acquire(ctx, executionKey{jobID: request.JobID(), qualification: e.browserSource.ephemeral}, scope)
	if err != nil {
		return finishUnstarted(err, !errors.Is(err, ndexec.ErrTreeNotDrained))
	}
	drained := true
	defer func() {
		if drained {
			e.release(job)
		}
	}()
	if e.isPoisoned() {
		return finishUnstarted(ndexec.ErrTreeNotDrained, false)
	}
	if err := ctx.Err(); err != nil {
		return finishUnstarted(err, true)
	}
	admittedAt := time.Now()
	if admitted != nil {
		schedule := admitted(admittedAt)
		run.ScheduledUS = schedule.DueUS
		run.QueuedUS = schedule.QueuedUS
	}
	run.StartedUS = admittedAt.UnixMicro()
	attempt, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	if err := synthetic.ValidateRequest(request); err != nil {
		return e.failAdmitted(&run, err, redact)
	}
	if err := e.platform(); err != nil {
		return e.failAdmitted(&run, err, redact)
	}
	use, err := e.beginUse()
	if err != nil {
		return e.failAdmitted(&run, err, redact)
	}
	cliDrained := true
	defer func() { _ = use.Finish(cliDrained) }()
	paths := use.Paths()
	if err := e.checkFiles(request.Kind, paths); err != nil {
		return e.failAdmitted(&run, err, redact)
	}
	if e.artifacts == nil {
		return e.failAdmitted(&run, errors.New("artifact owner is unavailable"), redact)
	}
	identity := paths.Identity
	run.Runtime = &identity
	e.save(attempt, "start", &run, redact)
	work, err := e.artifacts.Begin(run.ID, run.JobID, run.VnodeGUID)
	if err != nil {
		run.Error = redact.text("create run work directory: " + err.Error())
		e.complete(&run, nil, redact)
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	req := wireRequest{
		Version:    SupportedContract,
		Kind:       request.Kind,
		RunID:      run.ID,
		WorkDir:    work,
		Script:     request.Script,
		ScriptPath: request.ScriptPath,
		URL:        request.URL,
		TimeoutMS:  request.Timeout.Milliseconds(),
		Capture:    request.Capture,
		Secrets:    request.Secrets,
	}
	slot := job.slot
	// Keep browser startup and required retirement within attempt duration,
	// whether Chromium belongs to the CLI tree or a separate reusable owner.
	started := time.Now()
	req.Browser, err = e.prepareBrowser(attempt, slot, scope, paths)
	if err != nil {
		if errors.Is(err, ndexec.ErrTreeNotDrained) {
			drained = false
			return finishUndrained(&run, "browser owner retirement is unverified")
		}
		run.Error = redact.text(err.Error())
		switch {
		case errors.Is(err, context.Canceled):
			run.Outcome = synthetic.Cancelled
		case errors.Is(err, context.DeadlineExceeded):
			run.Outcome = synthetic.Timeout
		}
		e.complete(&run, []synthetic.Capture{}, redact)
		return synthetic.Execution{Run: run, Drained: true}
	}
	if req.Browser != nil {
		redact = req.Browser.redactor(request.Secrets)
	}
	raw, err := json.Marshal(req)
	if err == nil && len(raw) > FrameMaxBytes {
		err = errors.New("runner request exceeds 4 MiB transport limit")
	}
	if err != nil {
		if scope != nil && scope.ephemeral {
			drained = slot.retireOwner()
			if !drained {
				return finishUndrained(&run, "browser owner retirement is unverified")
			}
		}
		run.Error = redact.text(err.Error())
		e.complete(&run, []synthetic.Capture{}, redact)
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	sink := newSink(request.Kind, request.Capture, redact)
	sink.loan = req.Browser
	if state != nil {
		state("running")
	}
	result, startErr := e.executeProcess(
		attempt,
		paths,
		use.Completion(),
		[]string{paths.EntryPoint},
		raw,
		sink.consume,
		sink.stderr,
	)
	executionErr := attempt.Err()
	if startErr != nil {
		if req.Browser != nil {
			drained = slot.retireOwner()
			if !drained {
				return finishUndrained(&run, "browser owner retirement is unverified")
			}
		}
		if errors.Is(executionErr, context.Canceled) {
			run.Outcome = synthetic.Cancelled
		} else if errors.Is(executionErr, context.DeadlineExceeded) {
			run.Outcome = synthetic.Timeout
		}
		run.Error = redact.text(startErr.Error())
	} else {
		cliDrained = result.Drained
		drained = cliDrained
		snapshot := sink.snapshot()
		run.Events = snapshot.events
		run.DroppedEvents = snapshot.dropped
		run.Tests = snapshot.partial
		var browserErr error
		if !cliDrained && req.Browser != nil {
			// Fail-stop owns the remaining work; promptly stop the separate
			// browser supervisor without treating that request as a receipt.
			slot.owner.force()
		}
		if cliDrained && req.Browser != nil {
			drained, browserErr = slot.finishLoan(executionErr, snapshot)
		}
		elapsed := float64(time.Since(started)) / float64(time.Millisecond)
		run.DurationMS = &elapsed
		switch {
		case !drained:
			if !cliDrained {
				run.Error = redact.text("process-tree completion is unverified: " + errorText(result.Err))
			} else {
				run.Error = "browser owner retirement is unverified"
			}
		case errors.Is(executionErr, context.DeadlineExceeded):
			run.Outcome = synthetic.Timeout
			run.Error = redact.text("overall execution deadline exceeded")
		case errors.Is(executionErr, context.Canceled):
			run.Outcome = synthetic.Cancelled
			run.Error = redact.text("execution cancelled")
		case snapshot.err != nil:
			run.Error = redact.text(snapshot.err.Error())
		case snapshot.result == nil:
			run.Error = redact.text("runner exited without a terminal result: " + errorText(result.Err))
		case browserErr != nil && snapshot.result.Status != synthetic.Failed && snapshot.result.Status != synthetic.Error &&
			snapshot.result.Status != synthetic.Timeout && snapshot.result.Status != synthetic.Cancelled:
			run.Error = browserErr.Error()
		case snapshot.result.Status == synthetic.Success && result.Err != nil:
			run.Error = redact.text("runner exited unsuccessfully after reporting success")
		default:
			run.Outcome = snapshot.result.Status
			run.Error = redact.text(snapshot.result.Error)
			run.Tests = snapshot.result.Tests
			run.Metrics = snapshot.result.Metrics
			run.CaptureState = snapshot.result.CaptureState
		}
		if !drained {
			// Fatal was signalled before executeProcess joined the local pipe readers.
			// The command owner must fail-stop without closing stores or this work.
			return synthetic.Execution{
				Run:     run,
				Drained: false,
			}
		}
		captures := []synthetic.Capture{}
		if snapshot.err == nil && snapshot.result != nil {
			captures = append(captures, snapshot.result.Artifacts...)
		}
		e.complete(&run, captures, redact)
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	e.complete(&run, []synthetic.Capture{}, redact)
	return synthetic.Execution{
		Run:     run,
		Drained: true,
	}
}

func finishUndrained(run *synthetic.Run, message string) synthetic.Execution {
	run.Error = message
	return synthetic.Execution{Run: *run, Drained: false}
}

// A slot was admitted, so infrastructure/validation failure is an actual error
// observation. There was no child or workspace to finalize on this path.
func (e *Engine) failAdmitted(run *synthetic.Run, err error, redact *textRedactor) synthetic.Execution {
	run.Error = redact.text(err.Error())
	cleanup, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	e.save(cleanup, "start", run, redact)
	e.complete(run, nil, redact)
	return synthetic.Execution{Run: *run, Drained: true}
}

// complete owns a fixed cleanup budget independent of cancelled workflow input.
func (e *Engine) complete(run *synthetic.Run, captures []synthetic.Capture, redact *textRedactor) {
	cleanup, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if captures != nil {
		artifacts, err := e.artifacts.Finalize(cleanup, run.ID, captures)
		if err != nil {
			run.Error = joinText(run.Error, redact.text("artifact publication: "+err.Error()))
			if run.CaptureState != "disabled" {
				run.CaptureState = "unavailable"
			}
		} else {
			run.Artifacts = artifacts
			if run.CaptureState != "disabled" && len(artifacts) > 0 {
				run.CaptureState = "available"
			}
		}
	}
	run.CompletedUS = time.Now().UnixMicro()
	e.save(cleanup, "complete", run, redact)
}

func (e *Engine) save(ctx context.Context, phase string, run *synthetic.Run, redact *textRedactor) {
	if e.unrecorded {
		return
	}
	if e.history == nil {
		run.HistoryError = joinText(run.HistoryError, "history owner is unavailable")
		return
	}
	// A value snapshot is immutable; later completion must not alter start data.
	snapshot := synthetic.CloneRun(*run)
	attempted, err := e.history.AppendRun(ctx, phase, snapshot)
	if err != nil {
		qualifier := "not appended"
		if attempted {
			qualifier = "append outcome uncertain"
		}
		run.HistoryError = joinText(run.HistoryError, redact.text(phase+" history "+qualifier+": "+err.Error()))
	}
	if err = e.history.Sync(ctx); err != nil {
		run.HistoryError = joinText(run.HistoryError, redact.text(phase+" history sync: "+err.Error()))
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
