// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type processControl struct {
	ctx    context.Context
	result chan error
}

type processControls struct {
	restart   chan processControl
	terminate chan processControl
}

func newProcessControls() processControls {
	return processControls{
		restart:   make(chan processControl),
		terminate: make(chan processControl),
	}
}

func (pc processControls) valid() bool {
	return pc.restart != nil && pc.terminate != nil
}

type processInputCompletion struct {
	err  error
	quit bool
}

var errRunDidNotQuiesce = errors.New("jobmgr composition: run did not quiesce")

type processCoreConfig struct {
	Input           io.Reader                 // plugin stdin
	Output          io.Writer                 // plugin stdout
	ShutdownTimeout time.Duration             // per-run shutdown budget
	KeepAlive       bool                      // emit keepalive frames (long-lived agent mode)
	Modules         collectorapi.Registry     // collector module registry
	Jobs            runJobServices            // process-lifetime job services (resolver, catalogs, vnodes)
	Secrets         runSecretServices         // process-lifetime secret services
	Discovery       runDiscoveryServices      // discovery services (providers, build context)
	FinalizeOutput  func()                    // stops the runtime service at process teardown
	Diagnostics     jobmgr.DiagnosticObserver // process-wide operational log sink
}

type processCore struct {
	config      processCoreConfig         // retained process configuration
	diagnostics jobmgr.DiagnosticObserver // process-wide operational log sink

	uids        *lifecycle.UIDLedger            // process-lifetime UID ledger
	frames      *lifecycle.FrameOwner           // the one process-lifetime frame writer
	cleanupOut  *joboutput.CleanupOutputGate    // accepted-cleanup output until process finalization
	ingress     *functionadapter.ProcessIngress // the one process-lifetime stdin reader
	attempts    *containment.Authority          // process-lifetime opaque-work authority
	storeEpochs *processSecretEpochs            // process-owned per-run secret Store epochs
}

func newProcessCore(config processCoreConfig) (*processCore, error) {
	if config.Input == nil ||
		config.Output == nil ||
		config.ShutdownTimeout <= 0 ||
		config.Modules == nil ||
		config.Jobs.PluginName == "" ||
		config.Jobs.Defaults == nil ||
		config.Jobs.Resolver == nil ||
		config.Jobs.StoreCreators == nil ||
		config.Jobs.Vnodes == nil ||
		config.Diagnostics == nil ||
		!config.Discovery.valid() {
		return nil, errors.New("jobmgr composition: invalid process construction")
	}
	frames, err := lifecycle.NewFrameOwner(config.Output)
	if err != nil {
		return nil, err
	}
	cleanupOut, err := joboutput.NewCleanupOutputGate(frames)
	if err != nil {
		return nil, err
	}
	ingress, err := functionadapter.NewProcessIngress(config.Input)
	if err != nil {
		return nil, err
	}
	attempts, err := containment.NewAuthority(config.Diagnostics)
	if err != nil {
		return nil, err
	}
	storeEpochs, err := newProcessSecretEpochs(config.Jobs.Resolver, config.Diagnostics)
	if err != nil {
		return nil, err
	}
	return &processCore{
		config:      config,
		diagnostics: config.Diagnostics,
		uids:        lifecycle.NewUIDLedger(),
		frames:      frames,
		cleanupOut:  cleanupOut,
		ingress:     ingress,
		attempts:    attempts,
		storeEpochs: storeEpochs,
	}, nil
}

var errProcessTransitionInterrupted = errors.New("jobmgr composition: process transition interrupted")

type processTransitionResult struct {
	generation *runGeneration
	err        error
}

type processTransition struct {
	target  uint64
	control *processControl // nil only for the initial process startup
	cancel  context.CancelCauseFunc
	done    <-chan processTransitionResult
}

func (pc *processCore) run(ctx context.Context, controls processControls) error {
	if pc == nil || ctx == nil || !controls.valid() || pc.attempts == nil || pc.storeEpochs == nil {
		return errors.New("jobmgr composition: invalid process run")
	}
	generationID := uint64(1)
	var generation *runGeneration
	quit := false
	markQuit := func() { quit = true }
	transition := pc.beginStartTransition(ctx, ctx, generationID, markQuit, nil)
	var supersedingInitial *processControl
	var terminatingControl *processControl
	var terminalCause error
	terminating := false

	inputDone := make(chan processInputCompletion, 1)
	inputStarted := false
	startInput := func() {
		if inputStarted {
			return
		}
		inputStarted = true
		go func() {
			inputDone <- processInputCompletion{
				err:  pc.ingress.Run(ctx),
				quit: quit,
			}
		}()
	}
	ticks := ticker.New(time.Second)
	defer ticks.Stop()

	for {
		var transitionDone <-chan processTransitionResult
		var restartControls <-chan processControl
		var terminateControls <-chan processControl
		var activeInput <-chan processInputCompletion
		var kernelDone <-chan struct{}
		var processDone <-chan struct{}
		var tick <-chan int
		if transition != nil {
			transitionDone = transition.done
			if transition.control == nil &&
				supersedingInitial == nil &&
				!terminating {
				restartControls = controls.restart
			}
		} else if generation != nil && !terminating {
			restartControls = controls.restart
			kernelDone = generation.kernel.Done()
			tick = ticks.C
		}
		if !terminating {
			terminateControls = controls.terminate
			processDone = ctx.Done()
			if inputStarted {
				activeInput = inputDone
			}
		}

		select {
		case result := <-transitionDone:
			active := transition
			transition = nil
			if terminating {
				cause := terminalCause
				if result.err != nil && !processTransitionCancellationOnly(result.err) {
					cause = errors.Join(cause, result.err)
				}
				finalErr := pc.finalize(result.generation, cause)
				if terminalCause == nil && processTransitionCancellationOnly(finalErr) {
					finalErr = nil
				}
				if active.control != nil {
					active.control.result <- errors.Join(ErrProcessStopped, finalErr)
				}
				if supersedingInitial != nil {
					supersedingInitial.result <- errors.Join(ErrProcessStopped, finalErr)
				}
				if terminatingControl != nil {
					terminatingControl.result <- finalErr
				}
				return finalErr
			}

			if supersedingInitial != nil {
				control := supersedingInitial
				supersedingInitial = nil
				nextID := active.target + 1
				if nextID == 0 {
					finalErr := pc.finalize(
						result.generation,
						errors.New("jobmgr composition: run generation wrapped"),
					)
					control.result <- finalErr
					return finalErr
				}
				retireErr := pc.retireForSuccessor(control.ctx, result.generation, active.target, nextID)
				transitionErr := errors.Join(result.err, retireErr)
				if transitionErr != nil && !processTransitionCancellationOnly(transitionErr) {
					control.result <- processRestartResult(transitionErr)
					finalErr := pc.finalize(result.generation, transitionErr)
					return finalErr
				}
				generationID = nextID
				transition = pc.beginStartTransition(control.ctx, ctx, nextID, markQuit, control)
				continue
			}

			if result.err != nil {
				active.cancel(errProcessTransitionInterrupted)
				if active.control != nil && !processTransitionCancellationOnly(result.err) {
					active.control.result <- processRestartResult(result.err)
					active.control = nil
				}
				finalErr := pc.finalize(result.generation, result.err)
				if active.control != nil {
					active.control.result <- finalErr
				}
				return finalErr
			}
			generation = result.generation
			generationID = active.target
			startInput()
			if active.control != nil {
				active.control.result <- nil
			}
		case control := <-restartControls:
			if control.ctx == nil || control.result == nil {
				finalErr := pc.finalize(generation, errors.New("jobmgr composition: invalid restart control"))
				return finalErr
			}
			if transition != nil {
				supersedingInitial = &control
				transition.cancel(errProcessTransitionInterrupted)
				continue
			}
			nextID := generationID + 1
			if nextID == 0 {
				finalErr := pc.finalize(generation, errors.New("jobmgr composition: run generation wrapped"))
				control.result <- finalErr
				return finalErr
			}
			transition = pc.beginRotateTransition(control.ctx, ctx, generation, nextID, markQuit, control)
			generation = nil
		case control := <-terminateControls:
			if control.ctx == nil || control.result == nil {
				cause := errors.New("jobmgr composition: invalid terminate control")
				if transition == nil {
					return pc.finalize(generation, cause)
				}
				terminalCause = cause
			} else {
				terminatingControl = &control
			}
			terminating = true
			if transition == nil {
				finalErr := pc.finalize(generation, terminalCause)
				if terminatingControl != nil {
					terminatingControl.result <- finalErr
				}
				return finalErr
			}
			transition.cancel(errProcessTransitionInterrupted)
		case input := <-activeInput:
			terminating = true
			if !input.quit {
				terminalCause = errors.Join(
					errors.New("jobmgr composition: Function input stopped"),
					input.err,
				)
			} else {
				terminalCause = input.err
			}
			if transition == nil {
				return pc.finalize(generation, terminalCause)
			}
			transition.cancel(errProcessTransitionInterrupted)
		case <-kernelDone:
			return pc.finalize(
				generation,
				errors.Join(
					errors.New("jobmgr composition: active run stopped unexpectedly"),
					generation.Wait(context.Background()),
				),
			)
		case <-processDone:
			terminating = true
			terminalCause = ctx.Err()
			if transition == nil {
				return pc.finalize(generation, terminalCause)
			}
			transition.cancel(errProcessTransitionInterrupted)
		case clock := <-tick:
			if generation.run.IsStopping() {
				continue
			}
			if err := generation.scheduler.Tick(ctx, clock); err != nil {
				generation.run.Dirty(err)
				generation.Stop()
				continue
			}
			if pc.config.KeepAlive {
				if err := pc.frames.CommitProtocolFrame([]byte{'\n'}); err == nil {
					continue
				} else {
					return pc.finalize(
						generation,
						errors.Join(errors.New("jobmgr composition: keepalive write failed"), err),
					)
				}
			}
		}
	}
}

func processTransitionCancellationOnly(err error) bool {
	return ContainsOnlyProcessControlErrors(
		err,
		errProcessTransitionInterrupted,
		context.Canceled,
	)
}

func processRestartRecoveryDisposition(err error) error {
	var disposition error
	if processRotationTimeoutOnly(err) {
		disposition = context.DeadlineExceeded
	}
	if ContainsOnlyProcessControlErrors(
		err,
		errProcessTransitionInterrupted,
		context.Canceled,
		ErrProcessRestartRequired,
	) && errors.Is(err, ErrProcessRestartRequired) {
		disposition = errors.Join(disposition, ErrProcessRestartRequired)
	}
	return disposition
}

func processRestartResult(err error) error {
	// Restart owns only the transition result. Process-finalization failures
	// remain observable through Run after this known result is acknowledged.
	if disposition := processRestartRecoveryDisposition(err); disposition != nil {
		return disposition
	}
	return err
}

func processRotationTimeoutOnly(err error) bool {
	// The extra sentinels are consequences produced while enforcing the same
	// expired rotation budget. Any independent leaf still fails closed.
	return errors.Is(err, context.DeadlineExceeded) &&
		ContainsOnlyProcessControlErrors(
			err,
			errProcessTransitionInterrupted,
			context.Canceled,
			context.DeadlineExceeded,
			ErrProcessRestartRequired,
			jobmgr.ErrShutdownDeadlineExceeded,
			lifecycle.ErrRunTerminalNonQuiescent,
			errRunDidNotQuiesce,
		)
}

func (pc *processCore) beginStartTransition(
	startupParent context.Context,
	runParent context.Context,
	target uint64,
	quit func(),
	control *processControl,
) *processTransition {
	ctx, cancel := context.WithCancelCause(startupParent)
	done := make(chan processTransitionResult, 1)
	go func() {
		generation, err := pc.startGeneration(ctx, runParent, target, quit)
		done <- processTransitionResult{
			generation: generation,
			err:        err,
		}
	}()
	return &processTransition{
		target:  target,
		control: control,
		cancel:  cancel,
		done:    done,
	}
}

func (pc *processCore) beginRotateTransition(
	startupParent context.Context,
	runParent context.Context,
	current *runGeneration,
	target uint64,
	quit func(),
	control processControl,
) *processTransition {
	ctx, cancel := context.WithCancelCause(startupParent)
	done := make(chan processTransitionResult, 1)
	go func() {
		generation, err := pc.rotate(ctx, runParent, current, target, quit)
		done <- processTransitionResult{
			generation: generation,
			err:        err,
		}
	}()
	return &processTransition{
		target:  target,
		control: &control,
		cancel:  cancel,
		done:    done,
	}
}

func (pc *processCore) startGeneration(
	startupCtx context.Context,
	runCtx context.Context,
	generationID uint64,
	quit func(),
) (*runGeneration, error) {
	generation, err := pc.newRun(startupCtx, generationID)
	if err != nil {
		return nil, err
	}
	if err := generation.startWithRunContext(startupCtx, runCtx); err != nil {
		return generation, err
	}
	binding, err := pc.binding(generation, quit)
	if err != nil {
		return generation, err
	}
	if err := pc.ingress.Adopt(startupCtx, binding); err != nil {
		return generation, err
	}
	jobmgr.ObserveDiagnostic(pc.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticInfo,
		Name:       "job manager generation started",
		Generation: generationID,
	})
	return generation, nil
}

func (pc *processCore) newRun(
	ctx context.Context,
	generation uint64,
) (*runGeneration, error) {
	epoch, err := pc.storeEpochs.create(generation)
	if err != nil {
		return nil, err
	}
	run, err := newRunGeneration(ctx, runGenerationConfig{
		Generation:      generation,
		ShutdownTimeout: pc.config.ShutdownTimeout,
		Diagnostics:     pc.diagnostics,
		UIDs:            pc.uids,
		Frames:          pc.frames,
		CleanupOutput:   pc.cleanupOut,
		Modules:         pc.config.Modules,
		Jobs:            pc.config.Jobs,
		Secrets:         pc.config.Secrets,
		Discovery:       pc.config.Discovery,
		SecretEpoch:     epoch,
		Attempts:        pc.attempts,
	})
	if err != nil {
		return nil, errors.Join(err, pc.storeEpochs.seal(epoch))
	}
	return run, nil
}

func (pc *processCore) binding(generation *runGeneration, quit func()) (functionadapter.ProcessBinding, error) {
	if pc == nil || generation == nil || quit == nil {
		return functionadapter.ProcessBinding{}, errors.New("jobmgr composition: invalid process binding")
	}
	return functionadapter.NewProcessBinding(
		generation.kernel,
		generation.run.Generation(),
		lifecycle.RealClock{},
		quit,
	)
}

func (pc *processCore) rotate(
	startupCtx context.Context,
	runCtx context.Context,
	current *runGeneration,
	nextID uint64,
	quit func(),
) (*runGeneration, error) {
	jobmgr.ObserveDiagnostic(pc.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticInfo,
		Name:       "job manager generation rotation started",
		Generation: current.run.Generation(),
		State:      "rotating",
	})
	if err := pc.retireForSuccessor(startupCtx, current, current.run.Generation(), nextID); err != nil {
		return current, err
	}
	next, err := pc.startGeneration(startupCtx, runCtx, nextID, quit)
	if err != nil {
		if processRestartRequiresProcessExit(err) {
			return next, ErrProcessRestartRequired
		}
		return next, err
	}
	jobmgr.ObserveDiagnostic(pc.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticInfo,
		Name:       "job manager generation rotation completed",
		Generation: nextID,
		State:      "running",
	})
	return next, nil
}

func (pc *processCore) retireForSuccessor(
	ctx context.Context,
	current *runGeneration,
	retiringTarget uint64,
	nextID uint64,
) error {
	if ctx == nil {
		return errors.New("jobmgr composition: invalid successor retirement context")
	}
	if current != nil {
		if err := pc.storeEpochs.seal(current.secretEpoch); err != nil {
			pc.storeEpochs.observeFailure(
				current.run.Generation(),
				"secret Store epoch seal failed",
				err,
			)
			return err
		}
	}
	if pc.attempts != nil {
		pc.attempts.CutTarget(retiringTarget)
	}
	if current == nil {
		return nil
	}
	if !current.isStarted() {
		return current.abortConstruction()
	}
	ingressLive := pc.ingress.State() == functionadapter.ProcessIngressLive
	if ingressLive {
		if err := pc.ingress.SealPause(); err != nil {
			return err
		}
	}
	var budget *lifecycle.ShutdownBudget
	var err error
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.Cause(ctx)
		}
		budget, err = current.run.BeginShutdownWithTimeout(remaining)
	} else {
		budget, err = current.run.BeginShutdown()
	}
	if err != nil {
		return err
	}
	current.Stop()
	shutdownCtx := budget.Context()
	if ingressLive {
		if err := pc.ingress.DrainPause(shutdownCtx, nextID); err != nil {
			return err
		}
	}
	if err := pc.retireRun(shutdownCtx, current); err != nil {
		return err
	}
	return current.run.FinishShutdown()
}

func processRestartRequiresProcessExit(err error) bool {
	// A mixed tree is unexpected and must not be hidden behind a clean restart.
	return ContainsOnlyProcessControlErrors(
		err,
		jobmgr.ErrProcessAttemptQuarantined,
		jobmgr.ErrProcessAttemptWorkerPanic,
		jobmgr.ErrProcessAttemptFencePanic,
	)
}

func (pc *processCore) finalize(current *runGeneration, cause error) error {
	generation := uint64(0)
	if current != nil && current.run != nil {
		generation = current.run.Generation()
	}
	finalErr := cause
	if pc.attempts != nil {
		pc.attempts.BeginShutdown()
	}
	if pc.storeEpochs != nil {
		pc.storeEpochs.beginShutdown()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), pc.config.ShutdownTimeout)
	defer cancel()
	var finishRun *lifecycle.RunSupervisor
	if current != nil && current.isStarted() {
		if pc.ingress.State() == functionadapter.ProcessIngressLive {
			if err := pc.ingress.SealPause(); err != nil {
				finalErr = errors.Join(finalErr, err)
			}
		}
		current.Stop()
		budget, err := current.run.BeginShutdown()
		if err != nil {
			finalErr = errors.Join(finalErr, err)
		} else {
			shutdownCtx = budget.Context()
			finishRun = current.run
		}
		if pc.ingress.State() == functionadapter.ProcessIngressLive {
			if err := pc.ingress.DrainPause(shutdownCtx, 0); err != nil {
				finalErr = errors.Join(finalErr, err)
			}
		}
		if err := pc.retireRun(shutdownCtx, current); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	} else if current != nil {
		finalErr = errors.Join(finalErr, current.abortConstruction())
	}
	switch pc.ingress.State() {
	case functionadapter.ProcessIngressPaused:
		finalErr = errors.Join(finalErr, pc.ingress.Fence(shutdownCtx))
	case functionadapter.ProcessIngressContained:
	case functionadapter.ProcessIngressLive:
		finalErr = errors.Join(finalErr, errors.New("jobmgr composition: live Function ingress has no retiring run"))
	default:
		finalErr = errors.Join(finalErr, errors.New("jobmgr composition: invalid final Function ingress state"))
	}
	if pc.attempts != nil {
		if err := pc.attempts.Shutdown(shutdownCtx); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	pc.cleanupOut.Fence()
	if pc.storeEpochs != nil {
		if err := pc.storeEpochs.shutdown(shutdownCtx); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	if err := closeProcessUIDs(shutdownCtx, pc.uids); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	if finishRun != nil {
		if err := finishRun.FinishShutdown(); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	if pc.ingress.State() != functionadapter.ProcessIngressContained {
		finalErr = errors.Join(finalErr, errors.New("jobmgr composition: Function ingress was not contained"))
	}
	if pc.config.FinalizeOutput != nil {
		pc.config.FinalizeOutput()
	}
	level := jobmgr.DiagnosticInfo
	name := "job manager process stopped"
	if finalErr != nil {
		level = jobmgr.DiagnosticError
		name = "job manager process failed"
	}
	jobmgr.ObserveDiagnostic(pc.diagnostics, jobmgr.DiagnosticEvent{
		Level:      level,
		Name:       name,
		Generation: generation,
		Err:        finalErr,
	})
	return finalErr
}

func (pc *processCore) retireRun(ctx context.Context, generation *runGeneration) error {
	if generation == nil {
		return nil
	}
	waitErr := generation.Wait(ctx)
	terminal := generation.run.TerminalState()
	if !terminal.Reached || !terminal.Quiescent {
		err := errors.Join(
			waitErr,
			generation.run.DirtyCause(),
			errRunDidNotQuiesce,
		)
		jobmgr.ObserveDiagnostic(generation.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "job manager run did not quiesce",
			Generation: generation.run.Generation(),
			Err:        err,
		})
		return err
	}
	if generation.tasks.Active() != 0 ||
		generation.tasks.Pending() != 0 ||
		generation.tasks.InheritedActive() != 0 ||
		generation.tasks.LongLivedCensus() != (lifecycle.LongLivedCensus{}) {
		err := errors.Join(waitErr, errors.New("jobmgr composition: retired run retained tasks"))
		jobmgr.ObserveDiagnostic(generation.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "job manager retired run retained tasks",
			Generation: generation.run.Generation(),
			Err:        err,
		})
		return err
	}
	return waitErr
}

func closeProcessUIDs(ctx context.Context, uids *lifecycle.UIDLedger) error {
	if ctx == nil || uids == nil {
		return errors.New("jobmgr composition: invalid UID close")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := uids.CloseBatch(lifecycle.UIDReturnBatch)
		if err != nil {
			return err
		}
		if !more {
			return ctx.Err()
		}
	}
}
