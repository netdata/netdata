// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/ticker"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type processCommand uint8

const (
	processRestart processCommand = iota + 1
	processTerminate
)

type processControl struct {
	command processCommand
	result  chan error
}

type processInputCompletion struct {
	err  error
	quit bool
}

type processCoreConfig struct {
	Input           io.Reader
	Output          io.Writer
	Clock           lifecycle.Clock
	FirstGeneration uint64
	ShutdownTimeout time.Duration
	KeepAlive       bool
	Modules         collectorapi.Registry
	Jobs            runJobServices
	Secrets         runSecretServices
	Discovery       runDiscoveryServices
	Planner         runPlannerFactory
	FinalizeOutput  func()
}

type processCore struct {
	config processCoreConfig

	admission *lifecycle.AdmissionLedger
	uids      *lifecycle.UIDLedger
	frames    *lifecycle.FrameOwner
	ingress   *functionadapter.ProcessIngress
	quit      atomic.Bool

	mu      sync.Mutex
	started bool
}

const processAdmissionBytes = functionadapter.MaximumCatalogStorageBytes +
	lifecycle.TaskChildExecutionBytes

func newProcessCore(config processCoreConfig) (*processCore, error) {
	if config.Input == nil ||
		config.Output == nil ||
		config.FirstGeneration == 0 ||
		config.ShutdownTimeout <= 0 ||
		config.Modules == nil ||
		config.Jobs.PluginName == "" ||
		config.Jobs.Defaults == nil ||
		config.Jobs.Resolver == nil ||
		config.Jobs.StoreCreators == nil ||
		config.Jobs.Vnodes == nil ||
		!config.Discovery.valid() ||
		config.Planner == nil {
		return nil, errors.New("jobmgr composition: invalid process construction")
	}
	if config.Clock == nil {
		config.Clock = lifecycle.RealClock{}
	}
	admission := lifecycle.NewAdmissionLedger()
	if err := admission.ReserveProcessBytes(
		processAdmissionBytes,
	); err != nil {
		return nil, err
	}
	frames, err := lifecycle.NewFrameOwner(config.Output)
	if err != nil {
		return nil, errors.Join(
			err,
			admission.ReleaseProcessBytes(
				processAdmissionBytes,
			),
		)
	}
	ingress, err := functionadapter.NewProcessIngress(config.Input, admission)
	if err != nil {
		return nil, errors.Join(
			err,
			admission.ReleaseProcessBytes(
				processAdmissionBytes,
			),
		)
	}
	return &processCore{
		config: config, admission: admission,
		uids: lifecycle.NewUIDLedger(), frames: frames, ingress: ingress,
	}, nil
}

func (process *processCore) run(
	ctx context.Context,
	commands <-chan processControl,
) error {
	if process == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr composition: invalid process run")
	}
	process.mu.Lock()
	if process.started {
		process.mu.Unlock()
		return errors.New("jobmgr composition: process already started")
	}
	process.started = true
	process.mu.Unlock()

	generationID := process.config.FirstGeneration
	generation, err := process.newRun(generationID)
	if err != nil {
		return process.finalize(nil, generationID, err)
	}
	if err := generation.Start(ctx); err != nil {
		return process.finalize(generation, generationID, err)
	}
	binding, err := process.binding(generation)
	if err != nil {
		return process.finalize(generation, generationID, err)
	}
	if err := process.ingress.Adopt(ctx, binding); err != nil {
		return process.finalize(generation, generationID, err)
	}
	inputDone := make(chan processInputCompletion, 1)
	go func() {
		inputDone <- processInputCompletion{
			err:  process.ingress.Run(ctx),
			quit: process.quit.Load(),
		}
	}()
	ticks := ticker.New(time.Second)
	defer ticks.Stop()

	for {
		select {
		case input := <-inputDone:
			if input.quit {
				return process.finalize(generation, generationID, input.err)
			}
			return process.finalize(
				generation,
				generationID,
				errors.Join(
					errors.New("jobmgr composition: Function input stopped"),
					input.err,
				),
			)
		case <-generation.kernel.Done():
			return process.finalize(
				generation,
				generationID,
				errors.Join(
					errors.New("jobmgr composition: active run stopped unexpectedly"),
					generation.Wait(context.Background()),
				),
			)
		case <-ctx.Done():
			return process.finalize(generation, generationID, ctx.Err())
		case clock := <-ticks.C:
			if err := generation.scheduler.Tick(ctx, clock); err != nil {
				_ = generation.run.Dirty(err)
				generation.Stop()
				continue
			}
			if process.config.KeepAlive {
				if err := process.frames.CommitProtocolFrame(
					[]byte{'\n'},
				); err == nil {
					continue
				} else {
					return process.finalize(
						generation,
						generationID,
						errors.Join(
							errors.New(
								"jobmgr composition: keepalive write failed",
							),
							err,
						),
					)
				}
			}
		case control := <-commands:
			if control.result == nil {
				return process.finalize(
					generation,
					generationID,
					errors.New("jobmgr composition: invalid process control"),
				)
			}
			switch control.command {
			case processRestart:
				nextID := generationID + 1
				if nextID == 0 {
					finalErr := process.finalize(
						generation,
						generationID,
						errors.New("jobmgr composition: run generation wrapped"),
					)
					control.result <- finalErr
					return finalErr
				}
				next, finalGeneration, rotateErr := process.rotate(
					ctx,
					generation,
					generationID,
					nextID,
				)
				if rotateErr != nil {
					finalErr := process.finalize(next, finalGeneration, rotateErr)
					control.result <- finalErr
					return finalErr
				}
				generation = next
				generationID = nextID
				control.result <- nil
			case processTerminate:
				finalErr := process.finalize(generation, generationID, nil)
				control.result <- finalErr
				return finalErr
			default:
				finalErr := process.finalize(
					generation,
					generationID,
					errors.New("jobmgr composition: invalid process command"),
				)
				control.result <- finalErr
				return finalErr
			}
		}
	}
}

func (process *processCore) newRun(
	generation uint64,
) (*runGeneration, error) {
	return newRunGeneration(runGenerationConfig{
		Generation: generation, ShutdownTimeout: process.config.ShutdownTimeout,
		Clock: process.config.Clock, Admission: process.admission,
		UIDs: process.uids, Frames: process.frames,
		Modules: process.config.Modules, Jobs: process.config.Jobs,
		Secrets:   process.config.Secrets,
		Discovery: process.config.Discovery,
		Planner:   process.config.Planner,
	})
}

func (process *processCore) binding(
	generation *runGeneration,
) (functionadapter.ProcessBinding, error) {
	if process == nil || generation == nil {
		return functionadapter.ProcessBinding{},
			errors.New("jobmgr composition: invalid process binding")
	}
	return functionadapter.NewProcessBinding(
		generation.kernel,
		process.admission,
		generation.run.Generation(),
		generation.inputBodyGrants,
		process.config.Clock,
		func() {
			process.quit.Store(true)
		},
	)
}

func (process *processCore) rotate(
	ctx context.Context,
	current *runGeneration,
	currentID uint64,
	nextID uint64,
) (*runGeneration, uint64, error) {
	if err := process.ingress.SealPause(); err != nil {
		return current, currentID, err
	}
	current.Stop()
	budget, err := current.run.BeginShutdown()
	if err != nil {
		return current, currentID, err
	}
	shutdownCtx := budget.Context()
	if err := process.ingress.DrainPause(shutdownCtx, nextID); err != nil {
		return current, currentID, err
	}
	if err := process.retireRun(shutdownCtx, current); err != nil {
		return current, currentID, err
	}
	if err := current.run.FinishShutdown(); err != nil {
		return current, currentID, err
	}
	if err := process.admission.ReopenDrained(currentID, nextID); err != nil {
		return current, currentID, err
	}
	next, err := process.newRun(nextID)
	if err != nil {
		return nil, nextID, err
	}
	if err := next.Start(ctx); err != nil {
		return next, nextID, err
	}
	binding, err := process.binding(next)
	if err != nil {
		return next, nextID, err
	}
	if err := process.ingress.Adopt(ctx, binding); err != nil {
		return next, nextID, err
	}
	return next, nextID, nil
}

func (process *processCore) finalize(
	current *runGeneration,
	generation uint64,
	cause error,
) error {
	finalErr := cause
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		process.config.ShutdownTimeout,
	)
	defer cancel()
	var finishRun *lifecycle.RunSupervisor
	if current != nil && current.Started() {
		if process.ingress.Census().State == functionadapter.ProcessIngressLive {
			if err := process.ingress.SealPause(); err != nil {
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
		if census := process.ingress.Census(); census.State == functionadapter.ProcessIngressLive {
			if err := process.ingress.DrainPause(shutdownCtx, 0); err != nil {
				finalErr = errors.Join(finalErr, err)
			}
		}
		if err := process.retireRun(shutdownCtx, current); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	} else if current != nil {
		finalErr = errors.Join(finalErr, current.abortConstruction())
	}
	ingress := process.ingress.Census()
	switch ingress.State {
	case functionadapter.ProcessIngressPaused:
		finalErr = errors.Join(finalErr, process.ingress.Fence(shutdownCtx))
	case functionadapter.ProcessIngressContained:
	case functionadapter.ProcessIngressLive:
		finalErr = errors.Join(
			finalErr,
			errors.New("jobmgr composition: live Function ingress has no retiring run"),
		)
	default:
		finalErr = errors.Join(
			finalErr,
			errors.New("jobmgr composition: invalid final Function ingress state"),
		)
	}
	if err := closeProcessUIDs(shutdownCtx, process.uids); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	switch census := process.admission.Census(); census.Phase {
	case "ordinary-open":
		if err := process.admission.BeginCleanupOnly(generation); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
		fallthrough
	case "cleanup-only":
		if err := process.admission.CloseDrained(generation); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	case "closed":
	default:
		finalErr = errors.Join(
			finalErr,
			errors.New("jobmgr composition: invalid final admission phase"),
		)
	}
	if finishRun != nil {
		if err := finishRun.FinishShutdown(); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	if err := process.admission.ReleaseProcessBytes(
		processAdmissionBytes,
	); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	if process.ingress.Census().State != functionadapter.ProcessIngressContained {
		finalErr = errors.Join(
			finalErr,
			errors.New("jobmgr composition: Function ingress was not contained"),
		)
	}
	if process.config.FinalizeOutput != nil {
		process.config.FinalizeOutput()
	}
	return finalErr
}

func (process *processCore) retireRun(
	ctx context.Context,
	generation *runGeneration,
) error {
	if generation == nil {
		return nil
	}
	waitErr := generation.Wait(ctx)
	terminal := generation.run.TerminalState()
	if !terminal.Reached || !terminal.Quiescent {
		return errors.Join(
			waitErr,
			generation.run.DirtyCause(),
			errors.New("jobmgr composition: run did not quiesce"),
		)
	}
	if generation.tasks.Active() != 0 ||
		generation.tasks.Pending() != 0 ||
		generation.tasks.InheritedCensus().Active != 0 ||
		generation.tasks.LongLivedCensus() != (lifecycle.LongLivedCensus{}) {
		return errors.Join(
			waitErr,
			errors.New("jobmgr composition: retired run retained tasks"),
		)
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
