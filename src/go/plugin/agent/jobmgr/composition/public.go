// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

var ErrProcessStopped = errors.New("jobmgr composition: process stopped")

type RuntimeService interface {
	runtimecomp.Service
	Start(pluginName string, output io.Writer)
	Stop()
}

// Config is the process-fixed production composition input. NewProcess freezes
// the mutable registries and constructs the provider, secret-creator, resolver,
// vnode-metadata, admission, UID, and frame authorities exactly once.
type Config struct {
	Input  io.Reader
	Output io.Writer

	PluginName string
	Modules    collectorapi.Registry
	Defaults   confgroup.Registry

	DiscoveryBuildContext agentdiscovery.BuildContext
	DiscoveryProviders    []agentdiscovery.ProviderFactory
	RunJob                []string
	AutoEnable            bool

	SecretStoreCreators []secretstore.Creator
	Resolver            *secretresolver.AtomicResolver
	InitialSecrets      []secretstore.Config
	InitialVnodes       map[string]*vnodes.VirtualNode
	InitialJobs         []dyncfg.GraphConfig

	Runtime RuntimeService

	FirstGeneration uint64
	ShutdownTimeout time.Duration
	Clock           lifecycle.Clock
	KeepAlive       bool
}

// Process owns the one process-lifetime ingress and rotates only complete run
// generations. Restart and Terminate are acknowledged by Run returning from
// the resulting transition or final shutdown.
type Process struct {
	core     *processCore
	commands chan processControl
	started  chan struct{}
	done     chan struct{}

	mu        sync.Mutex
	attempted bool
	running   bool
	result    error

	runtime    RuntimeService
	pluginName string
}

func NewProcess(config Config) (*Process, error) {
	if config.Input == nil ||
		config.Output == nil ||
		config.PluginName == "" ||
		len(config.Modules) == 0 ||
		len(config.Defaults) == 0 ||
		len(config.DiscoveryProviders) == 0 {
		return nil, errors.New(
			"jobmgr composition: incomplete production configuration",
		)
	}
	modules := maps.Clone(config.Modules)
	defaults := maps.Clone(config.Defaults)
	providers, err := agentdiscovery.NewProviderCatalog(
		append(
			[]agentdiscovery.ProviderFactory(nil),
			config.DiscoveryProviders...,
		),
	)
	if err != nil {
		return nil, err
	}
	creators := append(
		[]secretstore.Creator(nil),
		config.SecretStoreCreators...,
	)
	if creators == nil {
		creators = backends.Creators()
	}
	creatorCatalog, err := secretstore.NewCreatorCatalog(creators)
	if err != nil {
		return nil, err
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver, err = secretresolver.NewDefaultAtomicResolver()
		if err != nil {
			return nil, err
		}
	}
	initialVnodes := make(
		map[string]*vnodes.VirtualNode,
		len(config.InitialVnodes),
	)
	for id, vnode := range config.InitialVnodes {
		if vnode == nil {
			return nil, fmt.Errorf(
				"jobmgr composition: initial vnode %q is nil",
				id,
			)
		}
		initialVnodes[id] = vnode.Copy()
	}
	initialJobs := make(
		[]dyncfg.GraphConfig,
		len(config.InitialJobs),
	)
	for index, record := range config.InitialJobs {
		record.Payload = append([]byte(nil), record.Payload...)
		initialJobs[index] = record
	}
	build := config.DiscoveryBuildContext
	if build.Identity.Name == "" {
		build.Identity.Name = config.PluginName
	}
	if build.Identity.Name != config.PluginName {
		return nil, errors.New(
			"jobmgr composition: discovery identity differs from plugin",
		)
	}
	build.Registry = defaults
	build.Out = config.Output
	build.FnReg = nil
	firstGeneration := config.FirstGeneration
	if firstGeneration == 0 {
		firstGeneration = 1
	}
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 10 * time.Second
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
		Input: config.Input, Output: config.Output,
		Clock: config.Clock, FirstGeneration: firstGeneration,
		ShutdownTimeout: shutdownTimeout,
		KeepAlive:       config.KeepAlive,
		Modules:         modules,
		Jobs: runJobServices{
			PluginName:    config.PluginName,
			Defaults:      defaults,
			Resolver:      resolver,
			StoreCreators: creatorCatalog,
			Runtime:       config.Runtime,
			Vnodes:        vnoderegistry.New(),
			InitialVnodes: initialVnodes,
			Graph:         initialJobs,
		},
		Secrets: runSecretServices{
			Initial: initialSecrets,
		},
		Discovery: runDiscoveryServices{
			BuildContext: build,
			Providers:    providers,
			RunJob:       append([]string(nil), config.RunJob...),
			AutoEnable:   config.AutoEnable,
		},
		Planner:        productionPlanner,
		FinalizeOutput: finalizeOutput,
	})
	if err != nil {
		return nil, err
	}
	return &Process{
		core: core, commands: make(chan processControl),
		started: make(chan struct{}), done: make(chan struct{}),
		runtime: config.Runtime, pluginName: config.PluginName,
	}, nil
}

func (process *Process) Run(ctx context.Context) error {
	if process == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid production run")
	}
	process.mu.Lock()
	if process.attempted {
		process.mu.Unlock()
		return errors.New("jobmgr composition: production run already attempted")
	}
	process.attempted = true
	process.running = true
	close(process.started)
	process.mu.Unlock()

	if process.runtime != nil {
		process.runtime.Start(
			process.pluginName,
			processFrameWriter{frames: process.core.frames},
		)
	}
	result := process.core.run(ctx, process.commands)
	process.mu.Lock()
	process.result = result
	process.running = false
	close(process.done)
	process.mu.Unlock()
	return result
}

func (process *Process) Restart(ctx context.Context) error {
	return process.send(ctx, processRestart)
}

func (process *Process) Terminate(ctx context.Context) error {
	return process.send(ctx, processTerminate)
}

func (process *Process) Done() <-chan struct{} {
	if process == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return process.done
}

func (process *Process) Wait(ctx context.Context) error {
	if process == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid production wait")
	}
	select {
	case <-process.done:
		process.mu.Lock()
		defer process.mu.Unlock()
		return process.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *Process) send(
	ctx context.Context,
	command processCommand,
) error {
	if process == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid process command")
	}
	select {
	case <-process.done:
		return ErrProcessStopped
	default:
	}
	select {
	case <-process.started:
	case <-process.done:
		return ErrProcessStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	result := make(chan error, 1)
	select {
	case process.commands <- processControl{
		command: command,
		result:  result,
	}:
	case <-process.done:
		return ErrProcessStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-process.done:
		select {
		case err := <-result:
			return err
		default:
		}
		process.mu.Lock()
		defer process.mu.Unlock()
		return process.result
	}
}

func cloneSecretConfigs(
	configs []secretstore.Config,
) ([]secretstore.Config, error) {
	cloned := make([]secretstore.Config, len(configs))
	for index, config := range configs {
		payload, err := yaml.Marshal(config)
		if err != nil {
			return nil, errors.Join(
				errors.New("jobmgr composition: clone initial secret configuration"),
				err,
			)
		}
		var clone secretstore.Config
		if err := yaml.Unmarshal(payload, &clone); err != nil {
			return nil, errors.Join(
				errors.New("jobmgr composition: clone initial secret configuration"),
				err,
			)
		}
		cloned[index] = clone
	}
	return cloned, nil
}

func productionPlanner(
	capabilities runPlannerCapabilities,
) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
	if capabilities.Tasks == nil ||
		capabilities.Functions == nil ||
		capabilities.Jobs == nil ||
		capabilities.DynCfg == nil ||
		capabilities.Graph == nil ||
		capabilities.StoreScope == nil ||
		capabilities.StoreCensus == nil {
		return nil, nil, errors.New(
			"jobmgr composition: incomplete production capabilities",
		)
	}
	return productionRejectingPlanner{},
		jobmgr.RunFinalizerFunc(
			func(context.Context, uint64) error { return nil },
		),
		nil
}

type productionRejectingPlanner struct{}

func (productionRejectingPlanner) Plan(
	jobmgr.Request,
) (jobmgr.WorkPlan, error) {
	return jobmgr.RejectionPlan(lifecycle.ControlBadRequest), nil
}

type processFrameWriter struct {
	frames *lifecycle.FrameOwner
}

func (writer processFrameWriter) Write(payload []byte) (int, error) {
	if writer.frames == nil {
		return 0, errors.New("jobmgr composition: runtime output has no frame owner")
	}
	if len(payload) == 0 {
		return 0, nil
	}
	if err := writer.frames.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}
