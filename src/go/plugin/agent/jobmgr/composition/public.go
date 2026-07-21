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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
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
	Input  io.Reader // plugin stdin
	Output io.Writer // plugin stdout

	PluginName string                // plugin name (go.d / ibm.d / scripts.d)
	Modules    collectorapi.Registry // enabled collector module registry
	Defaults   confgroup.Registry    // per-module config defaults

	DiscoveryBuildContext agentdiscovery.BuildContext      // discovery build context (paths, defaults)
	DiscoveryProviders    []agentdiscovery.ProviderFactory // discovery provider factories
	RunJob                []string                         // allow-list filter of job names (empty = allow all)
	AutoEnable            bool                             // publish discovered jobs as Running vs Accepted

	InitialSecrets []secretstore.Config           // initial secret store configs
	InitialVnodes  map[string]*vnodes.VirtualNode // file-configured vnodes

	Runtime RuntimeService // runtime service (charts/host-scope; nil disables runtime charts)

	ShutdownTimeout time.Duration // per-run shutdown budget
	KeepAlive       bool          // emit keepalive frames (long-lived agent mode)
}

// Process owns the one process-lifetime ingress and rotates only complete run
// generations. Restart and Terminate are acknowledged by Run returning from
// the resulting transition or final shutdown.
type Process struct {
	core     *processCore        // the process core (owns ledgers, ingress, frames)
	commands chan processControl // inbound Restart/Terminate controls
	started  chan struct{}       // closed once Run starts
	done     chan struct{}       // closed once Run returns

	mu        sync.Mutex // guards attempted/result
	attempted bool       // Run has been attempted (once)
	result    error      // terminal run result

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
		return nil, errors.New(
			"jobmgr composition: incomplete production configuration",
		)
	}
	modules := maps.Clone(config.Modules)
	defaults := maps.Clone(config.Defaults)
	providers, err := agentdiscovery.NewProviderCatalog(
		slices.Clone(config.DiscoveryProviders),
	)
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
		p.runtime.Start(
			p.pluginName,
			processFrameWriter{frames: p.core.frames},
		)
	}
	result := p.core.run(ctx, p.commands)
	p.mu.Lock()
	p.result = result
	close(p.done)
	p.mu.Unlock()
	return result
}

func (p *Process) Restart(ctx context.Context) error {
	return p.send(ctx, processRestart)
}

func (p *Process) Terminate(ctx context.Context) error {
	return p.send(ctx, processTerminate)
}

func (p *Process) send(
	ctx context.Context,
	command processCommand,
) error {
	if p == nil || ctx == nil {
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
	case p.commands <- processControl{
		command: command,
		result:  result,
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

type processFrameWriter struct {
	frames *lifecycle.FrameOwner
}

func (pfw processFrameWriter) Write(payload []byte) (int, error) {
	if pfw.frames == nil {
		return 0, errors.New("jobmgr composition: runtime output has no frame owner")
	}
	if len(payload) == 0 {
		return 0, nil
	}
	if err := pfw.frames.CommitBorrowedProtocolFrame(payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}
