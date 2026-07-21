// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
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
	Input           io.Reader             // plugin stdin
	Output          io.Writer             // plugin stdout
	ShutdownTimeout time.Duration         // per-run shutdown budget
	KeepAlive       bool                  // emit keepalive frames (long-lived agent mode)
	Modules         collectorapi.Registry // collector module registry
	Jobs            runJobServices        // process-lifetime job services (resolver, catalogs, vnodes)
	Secrets         runSecretServices     // process-lifetime secret services
	Discovery       runDiscoveryServices  // discovery services (providers, build context)
	FinalizeOutput  func()                // stops the runtime service at process teardown
}

type processRuntimeConfig struct {
	ShutdownTimeout time.Duration
	KeepAlive       bool
	Modules         collectorapi.Registry
	Jobs            runJobServices
	Secrets         runSecretServices
	Discovery       runDiscoveryServices
	FinalizeOutput  func()
}

type processCore struct {
	config processRuntimeConfig // retained process configuration

	admission *lifecycle.AdmissionLedger      // process-lifetime admission ledger
	uids      *lifecycle.UIDLedger            // process-lifetime UID ledger
	frames    *lifecycle.FrameOwner           // the one process-lifetime frame writer
	ingress   *functionadapter.ProcessIngress // the one process-lifetime stdin reader
	quit      atomic.Bool                     // set once to stop the outer loop
}

const processAdmissionBytes = functionadapter.MaximumCatalogStorageBytes

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
		!config.Discovery.valid() {
		return nil, errors.New("jobmgr composition: invalid process construction")
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
		config: processRuntimeConfig{
			ShutdownTimeout: config.ShutdownTimeout,
			KeepAlive:       config.KeepAlive,
			Modules:         config.Modules,
			Jobs:            config.Jobs,
			Secrets:         config.Secrets,
			Discovery:       config.Discovery,
			FinalizeOutput:  config.FinalizeOutput,
		},
		admission: admission,
		uids:      lifecycle.NewUIDLedger(), frames: frames, ingress: ingress,
	}, nil
}

func (pc *processCore) run(
	ctx context.Context,
	commands <-chan processControl,
) error {
	if pc == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr composition: invalid process run")
	}
	generationID := uint64(1)
	generation, err := pc.newRun(generationID)
	if err != nil {
		return pc.finalize(nil, generationID, err)
	}
	if err := generation.start(ctx); err != nil {
		return pc.finalize(generation, generationID, err)
	}
	binding, err := pc.binding(generation)
	if err != nil {
		return pc.finalize(generation, generationID, err)
	}
	if err := pc.ingress.Adopt(ctx, binding); err != nil {
		return pc.finalize(generation, generationID, err)
	}
	inputDone := make(chan processInputCompletion, 1)
	go func() {
		inputDone <- processInputCompletion{
			err:  pc.ingress.Run(ctx),
			quit: pc.quit.Load(),
		}
	}()
	ticks := ticker.New(time.Second)
	defer ticks.Stop()

	for {
		select {
		case input := <-inputDone:
			if input.quit {
				return pc.finalize(generation, generationID, input.err)
			}
			return pc.finalize(
				generation,
				generationID,
				errors.Join(
					errors.New("jobmgr composition: Function input stopped"),
					input.err,
				),
			)
		case <-generation.kernel.Done():
			return pc.finalize(
				generation,
				generationID,
				errors.Join(
					errors.New("jobmgr composition: active run stopped unexpectedly"),
					generation.Wait(context.Background()),
				),
			)
		case <-ctx.Done():
			return pc.finalize(generation, generationID, ctx.Err())
		case clock := <-ticks.C:
			if generation.run.IsStopping() {
				continue
			}
			if err := generation.scheduler.Tick(ctx, clock); err != nil {
				generation.run.Dirty(err)
				generation.Stop()
				continue
			}
			if pc.config.KeepAlive {
				if err := pc.frames.CommitProtocolFrame(
					[]byte{'\n'},
				); err == nil {
					continue
				} else {
					return pc.finalize(
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
				return pc.finalize(
					generation,
					generationID,
					errors.New("jobmgr composition: invalid process control"),
				)
			}
			switch control.command {
			case processRestart:
				nextID := generationID + 1
				if nextID == 0 {
					finalErr := pc.finalize(
						generation,
						generationID,
						errors.New("jobmgr composition: run generation wrapped"),
					)
					control.result <- finalErr
					return finalErr
				}
				next, finalGeneration, rotateErr := pc.rotate(
					ctx,
					generation,
					generationID,
					nextID,
				)
				if rotateErr != nil {
					finalErr := pc.finalize(next, finalGeneration, rotateErr)
					control.result <- finalErr
					return finalErr
				}
				generation = next
				generationID = nextID
				control.result <- nil
			case processTerminate:
				finalErr := pc.finalize(generation, generationID, nil)
				control.result <- finalErr
				return finalErr
			default:
				finalErr := pc.finalize(
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

func (pc *processCore) newRun(
	generation uint64,
) (*runGeneration, error) {
	return newRunGeneration(runGenerationConfig{
		Generation: generation, ShutdownTimeout: pc.config.ShutdownTimeout,
		Admission: pc.admission, UIDs: pc.uids, Frames: pc.frames,
		Modules: pc.config.Modules, Jobs: pc.config.Jobs,
		Secrets:   pc.config.Secrets,
		Discovery: pc.config.Discovery,
	})
}

func (pc *processCore) binding(
	generation *runGeneration,
) (functionadapter.ProcessBinding, error) {
	if pc == nil || generation == nil {
		return functionadapter.ProcessBinding{},
			errors.New("jobmgr composition: invalid process binding")
	}
	return functionadapter.NewProcessBinding(
		generation.kernel,
		pc.admission,
		generation.run.Generation(),
		generation.inputBodyGrants,
		lifecycle.RealClock{},
		func() {
			pc.quit.Store(true)
		},
	)
}

func (pc *processCore) rotate(
	ctx context.Context,
	current *runGeneration,
	currentID uint64,
	nextID uint64,
) (*runGeneration, uint64, error) {
	if err := pc.ingress.SealPause(); err != nil {
		return current, currentID, err
	}
	current.Stop()
	budget, err := current.run.BeginShutdown()
	if err != nil {
		return current, currentID, err
	}
	shutdownCtx := budget.Context()
	if err := pc.ingress.DrainPause(shutdownCtx, nextID); err != nil {
		return current, currentID, err
	}
	if err := pc.retireRun(shutdownCtx, current); err != nil {
		return current, currentID, err
	}
	if err := current.run.FinishShutdown(); err != nil {
		return current, currentID, err
	}
	if err := pc.admission.ReopenDrained(currentID, nextID); err != nil {
		return current, currentID, err
	}
	next, err := pc.newRun(nextID)
	if err != nil {
		return nil, nextID, err
	}
	if err := next.start(ctx); err != nil {
		return next, nextID, err
	}
	binding, err := pc.binding(next)
	if err != nil {
		return next, nextID, err
	}
	if err := pc.ingress.Adopt(ctx, binding); err != nil {
		return next, nextID, err
	}
	return next, nextID, nil
}

func (pc *processCore) finalize(
	current *runGeneration,
	generation uint64,
	cause error,
) error {
	finalErr := cause
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		pc.config.ShutdownTimeout,
	)
	defer cancel()
	var finishRun *lifecycle.RunSupervisor
	if current != nil && current.isStarted() {
		if pc.ingress.Census().State == functionadapter.ProcessIngressLive {
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
		if census := pc.ingress.Census(); census.State == functionadapter.ProcessIngressLive {
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
	ingress := pc.ingress.Census()
	switch ingress.State {
	case functionadapter.ProcessIngressPaused:
		finalErr = errors.Join(finalErr, pc.ingress.Fence(shutdownCtx))
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
	if err := closeProcessUIDs(shutdownCtx, pc.uids); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	switch census := pc.admission.Census(); census.Phase {
	case lifecycle.AdmissionOrdinaryOpen:
		if err := pc.admission.BeginCleanupOnly(generation); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
		fallthrough
	case lifecycle.AdmissionCleanupOnly:
		if err := pc.admission.CloseDrained(generation); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	case lifecycle.AdmissionClosed:
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
	if err := pc.admission.ReleaseProcessBytes(
		processAdmissionBytes,
	); err != nil {
		finalErr = errors.Join(finalErr, err)
	}
	if pc.ingress.Census().State != functionadapter.ProcessIngressContained {
		finalErr = errors.Join(
			finalErr,
			errors.New("jobmgr composition: Function ingress was not contained"),
		)
	}
	if pc.config.FinalizeOutput != nil {
		pc.config.FinalizeOutput()
	}
	return finalErr
}

func (pc *processCore) retireRun(
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
