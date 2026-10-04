// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner owns admission and supervised execution of prepared DEM assets.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
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
	Begin(string) (string, error)
	Finalize(context.Context, string, []synthetic.Capture) ([]synthetic.Artifact, error)
}

type Engine struct {
	config     Config
	history    historyWriter
	artifacts  ArtifactStore
	admission  chan struct{}
	fatal      chan error
	poisoned   chan struct{}
	poisonOnce sync.Once
	start      startFunc
	platform   func() error
}

func New(config Config, history historyWriter, artifacts ArtifactStore) *Engine {
	return &Engine{
		config:    config,
		history:   history,
		artifacts: artifacts,
		admission: make(chan struct{}, 1),
		fatal:     make(chan error, 1),
		poisoned:  make(chan struct{}),
		platform:  runtimeAllowed,
		start: func(ctx context.Context, opts ndexec.ProcessOptions, bin string, args ...string) (treeProcess, error) {
			return ndexec.StartUnprivilegedProcessTree(ctx, opts, bin, args...)
		},
	}
}

func (e *Engine) Fatal() <-chan error { return e.fatal }

func (e *Engine) poison(err error) {
	e.poisonOnce.Do(func() { close(e.poisoned); e.fatal <- errors.Join(ndexec.ErrTreeNotDrained, err) })
}

func (e *Engine) isPoisoned() bool {
	select {
	case <-e.poisoned:
		return true
	default:
		return false
	}
}

func (e *Engine) Execute(ctx context.Context, request synthetic.Request, state func(string)) synthetic.Execution {
	redact := newRedactor(request.Secrets)
	run := synthetic.Run{
		JobID:        request.JobID(),
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
	select {
	case <-ctx.Done():
		return finishUnstarted(ctx.Err(), true)
	case <-e.poisoned:
		return finishUnstarted(ndexec.ErrTreeNotDrained, false)
	case e.admission <- struct{}{}:
	}
	drained := true
	defer func() {
		if drained {
			<-e.admission
		}
	}()
	if e.isPoisoned() {
		drained = false
		return finishUnstarted(ndexec.ErrTreeNotDrained, false)
	}
	if err := ctx.Err(); err != nil {
		return finishUnstarted(err, true)
	}
	run.StartedUS = time.Now().UnixMicro()
	attempt, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	if err := synthetic.ValidateRequest(request); err != nil {
		return finishUnstarted(err, true)
	}
	if err := e.platform(); err != nil {
		return finishUnstarted(err, true)
	}
	if err := e.checkFiles(request.Kind); err != nil {
		return finishUnstarted(err, true)
	}
	if e.artifacts == nil {
		return finishUnstarted(errors.New("artifact owner is unavailable"), true)
	}
	e.save(attempt, "start", &run, redact)
	work, err := e.artifacts.Begin(run.ID)
	if err != nil {
		run.Error = redact.text("create run work directory: " + err.Error())
		e.complete(&run, nil, redact)
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	req := wireRequest{
		Version:          1,
		Kind:             request.Kind,
		RunID:            run.ID,
		DependenciesPath: e.config.DependenciesPath,
		BrowserPath:      e.config.BrowserPath,
		WorkDir:          work,
		Script:           request.Script,
		ScriptPath:       request.ScriptPath,
		URL:              request.URL,
		TimeoutMS:        request.Timeout.Milliseconds(),
		Capture:          request.Capture,
		Secrets:          request.Secrets,
	}
	raw, err := json.Marshal(req)
	if err == nil && len(raw) > FrameMaxBytes {
		err = errors.New("runner request exceeds 4 MiB transport limit")
	}
	if err != nil {
		run.Error = redact.text(err.Error())
		e.complete(&run, []synthetic.Capture{}, redact)
		return synthetic.Execution{
			Run:     run,
			Drained: true,
		}
	}
	sink := newSink(request.Kind, request.Capture, redact)
	started := time.Now()
	if state != nil {
		state("running")
	}
	result, startErr := e.executeProcess(
		attempt,
		[]string{filepath.Join(e.config.AssetsPath, "run.mjs")},
		raw,
		sink.consume,
		sink.stderr,
	)
	if startErr != nil {
		if errors.Is(attempt.Err(), context.Canceled) {
			run.Outcome = synthetic.Cancelled
		} else if errors.Is(attempt.Err(), context.DeadlineExceeded) {
			run.Outcome = synthetic.Timeout
		}
		run.Error = redact.text(startErr.Error())
	} else {
		elapsed := float64(time.Since(started)) / float64(time.Millisecond)
		run.DurationMS = &elapsed
		drained = result.Drained
		snapshot := sink.snapshot()
		run.Events = snapshot.events
		run.DroppedEvents = snapshot.dropped
		run.Tests = snapshot.partial
		switch {
		case !drained:
			run.Error = redact.text("process-tree completion is unverified: " + errorText(result.Err))
		case errors.Is(attempt.Err(), context.DeadlineExceeded):
			run.Outcome = synthetic.Timeout
			run.Error = redact.text("overall execution deadline exceeded")
		case errors.Is(attempt.Err(), context.Canceled):
			run.Outcome = synthetic.Cancelled
			run.Error = redact.text("execution cancelled")
		case snapshot.err != nil:
			run.Error = redact.text(snapshot.err.Error())
		case snapshot.result == nil:
			run.Error = redact.text("runner exited without a terminal result: " + errorText(result.Err))
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
