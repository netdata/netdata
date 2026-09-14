// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

var (
	ErrProcessStopped         = errors.New("jobmgr composition: process stopped")
	ErrProcessRestartRequired = errors.New("jobmgr composition: process restart required")
)

// ContainsOnlyProcessControlErrors reports whether every leaf in err matches
// one of the allowed control dispositions. Mixed and malformed trees fail
// closed.
func ContainsOnlyProcessControlErrors(err error, allowed ...error) bool {
	return jobmgr.ContainsOnlyErrorLeaves(err, allowed...)
}

// ProcessService runs alongside all run generations. It must return when ctx
// is canceled and handle its own operational failures without stopping jobs.
type ProcessService interface {
	Run(context.Context)
}

// ProcessServiceFinalizer optionally saves pending state after Run has joined,
// before jobs are retired. Finalize is called once with the process shutdown
// budget. It must honor cancellation; the process stops waiting at the deadline.
type ProcessServiceFinalizer interface {
	Finalize(context.Context) error
}

type RuntimeService interface {
	runtimecomp.Service
	Start(pluginName string, output io.Writer)
	Stop()
}

// Config is the process-fixed production composition input. NewProcess freezes
// the mutable registries and constructs the provider, secret-creator, resolver,
// vnode-metadata, UID, and frame authorities exactly once.
type Config struct {
	SNMPVnodeAcquirer vnodes.SNMPAcquirer
	Input             io.Reader // plugin stdin
	Output            io.Writer // plugin stdout

	PluginName string                // plugin name (go.d / ibm.d / scripts.d)
	Modules    collectorapi.Registry // enabled collector module registry
	Defaults   confgroup.Registry    // per-module config defaults

	DiscoveryBuildContext agentdiscovery.BuildContext      // discovery build context (paths, defaults)
	DiscoveryProviders    []agentdiscovery.ProviderFactory // discovery provider factories
	RunJob                []string                         // allow-list filter of job names (empty = allow all)
	AutoEnable            bool                             // publish discovered jobs as Running vs Accepted

	InitialSecrets []secretstore.Config      // initial secret store configs
	InitialVnodes  map[string]*vnodes.Config // file-configured vnodes

	Services []ProcessService // optional process-owned background services

	Runtime RuntimeService // runtime service (charts/host-scope; nil disables runtime charts)

	ShutdownTimeout time.Duration // per-run shutdown budget
	KeepAlive       bool          // emit keepalive frames (long-lived agent mode)
}

// Process owns the one process-lifetime ingress and rotates only complete run
// generations. Restart and Terminate are acknowledged by Run returning from
// the resulting transition or final shutdown.
type Process struct {
	core     *processCore    // the process core (owns ledgers, ingress, frames)
	controls processControls // independent Restart/Terminate delivery
	started  chan struct{}   // closed once Run starts
	done     chan struct{}   // closed once Run returns

	mu        sync.Mutex // guards attempted/result
	attempted bool       // Run has been attempted (once)
	result    error      // terminal run result

	services   []ProcessService
	runtime    RuntimeService // runtime service (started/stopped around Run)
	pluginName string         // plugin name
}

func NewProcess(config Config) (*Process, error) {
	if config.Input == nil ||
		config.Output == nil ||
		config.PluginName == "" ||
		len(config.Modules) == 0 ||
		len(config.Defaults) == 0 ||
		len(config.DiscoveryProviders) == 0 {
		return nil, errors.New("jobmgr composition: incomplete production configuration")
	}
	modules := maps.Clone(config.Modules)
	defaults := maps.Clone(config.Defaults)
	providers, err := agentdiscovery.NewProviderCatalog(slices.Clone(config.DiscoveryProviders))
	if err != nil {
		return nil, err
	}
	creatorCatalog, err := secretstore.NewCreatorCatalog(backends.Creators())
	if err != nil {
		return nil, err
	}
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	if err != nil {
		return nil, err
	}
	initialVnodes := make(map[string]*vnodes.Config, len(config.InitialVnodes))
	for id, vnode := range config.InitialVnodes {
		if vnode == nil {
			return nil, fmt.Errorf("jobmgr composition: initial vnode %q is nil", id)
		}
		if vnode.IsSNMP() && config.SNMPVnodeAcquirer == nil {
			return nil, fmt.Errorf("SNMP vnode mode is unavailable in this plugin")
		}
		initialVnodes[id] = vnode.Copy()
	}
	if err := validateInitialVNodeSet(initialVnodes); err != nil {
		return nil, err
	}
	build := config.DiscoveryBuildContext
	if build.Identity.Name == "" {
		build.Identity.Name = config.PluginName
	}
	if build.Identity.Name != config.PluginName {
		return nil, errors.New("jobmgr composition: discovery identity differs from plugin")
	}
	build.Registry = defaults
	build.DyncfgOutput = nil
	build.FnReg = nil
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = lifecycle.DefaultShutdownTimeout
	}
	initialSecrets, err := cloneSecretConfigs(config.InitialSecrets)
	if err != nil {
		return nil, err
	}
	var finalizeOutput func()
	if config.Runtime != nil {
		finalizeOutput = config.Runtime.Stop
	}
	core, err := newProcessCore(processCoreConfig{
		Input:           config.Input,
		Output:          config.Output,
		ShutdownTimeout: shutdownTimeout,
		KeepAlive:       config.KeepAlive,
		Modules:         modules,
		Jobs: runJobServices{
			PluginName:        config.PluginName,
			Defaults:          defaults,
			Resolver:          resolver,
			StoreCreators:     creatorCatalog,
			Runtime:           config.Runtime,
			InitialVnodes:     initialVnodes,
			SNMPVnodeAcquirer: config.SNMPVnodeAcquirer,
		},
		Secrets: runSecretServices{
			Initial: initialSecrets,
		},
		Discovery: runDiscoveryServices{
			BuildContext: build,
			Providers:    providers,
			RunJob:       slices.Clone(config.RunJob),
			AutoEnable:   config.AutoEnable,
		},
		FinalizeOutput: finalizeOutput,
		Diagnostics:    newProcessDiagnosticLogger(),
	})
	if err != nil {
		return nil, err
	}
	return &Process{
		core:       core,
		controls:   newProcessControls(),
		started:    make(chan struct{}),
		done:       make(chan struct{}),
		runtime:    config.Runtime,
		services:   slices.Clone(config.Services),
		pluginName: config.PluginName,
	}, nil
}

func (p *Process) Run(ctx context.Context) error {
	if p == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid production run")
	}
	p.mu.Lock()
	if p.attempted {
		p.mu.Unlock()
		return errors.New("jobmgr composition: production run already attempted")
	}
	p.attempted = true
	close(p.started)
	p.mu.Unlock()

	if p.runtime != nil {
		p.runtime.Start(p.pluginName, joboutput.FrameWriter{
			Owner: p.core.frames,
		})
	}
	stopServices := p.startServices(ctx)
	stop := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), p.core.config.ShutdownTimeout)
		defer cancel()
		return stopServices(shutdownCtx)
	}
	defer stop()
	p.core.config.StopServices = stopServices
	result := p.core.run(ctx, p.controls)
	if err := stop(); err != nil && !errors.Is(result, err) {
		result = errors.Join(result, err)
	}
	p.mu.Lock()
	p.result = result
	close(p.done)
	p.mu.Unlock()
	return result
}

func (p *Process) Restart(ctx context.Context) error {
	if p == nil {
		return errors.New("jobmgr composition: invalid process command")
	}
	return p.send(ctx, p.controls.restart)
}

func (p *Process) Terminate(ctx context.Context) error {
	if p == nil {
		return errors.New("jobmgr composition: invalid process command")
	}
	return p.send(ctx, p.controls.terminate)
}

func (p *Process) send(ctx context.Context, controls chan<- processControl) error {
	if p == nil || ctx == nil || controls == nil {
		return errors.New("jobmgr composition: invalid process command")
	}
	select {
	case <-p.done:
		return ErrProcessStopped
	default:
	}
	select {
	case <-p.started:
	case <-p.done:
		return ErrProcessStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	result := make(chan error, 1)
	select {
	case controls <- processControl{
		ctx:    ctx,
		result: result,
	}:
	case <-p.done:
		return ErrProcessStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-p.done:
		select {
		case err := <-result:
			return err
		default:
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.result
	}
}

func cloneSecretConfigs(configs []secretstore.Config) ([]secretstore.Config, error) {
	cloned := make([]secretstore.Config, len(configs))
	for index, config := range configs {
		payload, err := yaml.Marshal(config)
		if err != nil {
			return nil, errors.Join(errors.New("jobmgr composition: clone initial secret configuration"), err)
		}
		var clone secretstore.Config
		if err := yaml.Unmarshal(payload, &clone); err != nil {
			return nil, errors.Join(errors.New("jobmgr composition: clone initial secret configuration"), err)
		}
		cloned[index] = clone
	}
	return cloned, nil
}

func (p *Process) startServices(ctx context.Context) func(context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	type runningService struct {
		service ProcessService
		done    <-chan struct{}
	}
	var running []runningService
	for _, service := range p.services {
		if service == nil {
			continue
		}
		done := make(chan struct{})
		running = append(running, runningService{service, done})
		go func() {
			defer close(done)
			defer func() {
				if recovered := recover(); recovered != nil {
					jobmgr.ObserveDiagnostic(p.core.diagnostics, jobmgr.DiagnosticEvent{
						Level: jobmgr.DiagnosticError,
						Name:  "process service panicked",
						Err:   fmt.Errorf("%v", recovered),
					})
				}
			}()
			service.Run(ctx)
		}()
	}
	var once sync.Once
	var stopErr error
	return func(shutdownCtx context.Context) error {
		once.Do(func() {
			cancel()
			// Finalize independent services concurrently under the same budget.
			// A stuck Run must never overlap its own finalizer.
			var completions []<-chan error
			for _, entry := range running {
				result := make(chan error, 1)
				completions = append(completions, result)
				go func() { result <- finalizeProcessService(shutdownCtx, entry.service, entry.done) }()
			}
			for index, done := range completions {
				select {
				case err := <-done:
					stopErr = errors.Join(stopErr, err)
					continue
				default:
				}
				select {
				case err := <-done:
					stopErr = errors.Join(stopErr, err)
				case <-shutdownCtx.Done():
					stopErr = errors.Join(
						stopErr,
						fmt.Errorf("jobmgr composition: process services shutdown: %w", shutdownCtx.Err()),
					)
					// Preserve completed failures without waiting beyond the shared budget.
					for _, pending := range completions[index:] {
						select {
						case err := <-pending:
							stopErr = errors.Join(stopErr, err)
						default:
						}
					}
					return
				}
			}
		})
		return stopErr
	}
}

// A finalizer never overlaps its own Run; the caller bounds the wait even for
// service code or filesystem operations that do not respond to cancellation.
func finalizeProcessService(ctx context.Context, service ProcessService, done <-chan struct{}) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("process service finalization panicked: %v", recovered)
		}
	}()
	select {
	case <-done:
	default:
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("jobmgr composition: process services shutdown: %w", ctx.Err())
		}
	}
	if finalizer, ok := service.(ProcessServiceFinalizer); ok {
		if err := ctx.Err(); err != nil {
			return err
		}
		return finalizer.Finalize(ctx)
	}
	return nil
}
