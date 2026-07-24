// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/runtimechartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var ErrNotRunning = errors.New("agent: process is not running")

// Config is an Agent configuration.
type Config struct {
	Name            string
	PluginConfigDir []string

	CollectorsConfigDir       []string
	CollectorsConfigWatchPath []string
	ServiceDiscoveryConfigDir []string
	VarLibDir                 string

	ModuleRegistry  collectorapi.Registry
	RunModule       string
	RunJob          []string
	MinUpdateEvery  int
	ShutdownTimeout time.Duration

	DisableServiceDiscovery bool

	IsInsideK8s bool

	RunModePolicy policy.RunModePolicy

	DiscoveryProviders []discovery.ProviderFactory
}

// Agent represents orchestrator.
type Agent struct {
	*logger.Logger

	Name string

	ConfigDir                 multipath.MultiPath
	CollectorsConfDir         multipath.MultiPath
	CollectorsConfigWatchPath []string
	ServiceDiscoveryConfigDir multipath.MultiPath

	VarLibDir string

	RunModule       string
	RunJob          []string
	MinUpdateEvery  int
	ShutdownTimeout time.Duration

	DisableServiceDiscovery bool

	IsInsideK8s bool

	runModePolicy policy.RunModePolicy

	DiscoveryProviders []discovery.ProviderFactory

	ModuleRegistry collectorapi.Registry
	In             io.Reader
	Out            io.Writer

	processMu    sync.Mutex
	process      *composition.Process
	processReady chan struct{}
	readyOnce    sync.Once
}

// New creates a new Agent.
func New(cfg Config) *Agent {
	a := &Agent{
		Logger: logger.New().With(
			slog.String("component", "agent"),
		),
		Name:                      cfg.Name,
		ConfigDir:                 cfg.PluginConfigDir,
		CollectorsConfDir:         cfg.CollectorsConfigDir,
		ServiceDiscoveryConfigDir: cfg.ServiceDiscoveryConfigDir,
		CollectorsConfigWatchPath: cfg.CollectorsConfigWatchPath,
		VarLibDir:                 cfg.VarLibDir,
		RunModule:                 cfg.RunModule,
		RunJob:                    cfg.RunJob,
		MinUpdateEvery:            cfg.MinUpdateEvery,
		ShutdownTimeout:           cfg.ShutdownTimeout,
		IsInsideK8s:               cfg.IsInsideK8s,
		runModePolicy:             cfg.RunModePolicy,
		ModuleRegistry:            cfg.ModuleRegistry,
		DiscoveryProviders:        cfg.DiscoveryProviders,
		In:                        os.Stdin,
		Out:                       safewriter.Stdout,
		DisableServiceDiscovery:   cfg.DisableServiceDiscovery,
		processReady:              make(chan struct{}),
	}

	return a
}

// RunContext runs one agent instance lifecycle on the provided context.
func (a *Agent) RunContext(ctx context.Context) error {
	return a.run(ctx)
}

func (a *Agent) Restart(ctx context.Context) error {
	process, err := a.awaitProcess(ctx)
	if err != nil {
		return err
	}
	return normalizeProcessControlError(process.Restart(ctx))
}

func (a *Agent) Terminate(ctx context.Context) error {
	process, err := a.awaitProcess(ctx)
	if err != nil {
		return err
	}
	return normalizeProcessControlError(process.Terminate(ctx))
}

func normalizeProcessControlError(err error) error {
	if errors.Is(err, composition.ErrProcessStopped) {
		return ErrNotRunning
	}
	return err
}

func (a *Agent) run(ctx context.Context) error {
	a.Info("instance is started")
	defer func() { a.Info("instance is stopped") }()
	defer a.markProcessReady()

	cfg := a.loadPluginConfig()
	a.Infof("using config: %s", cfg.String())

	if !cfg.Enabled {
		a.Info("plugin is disabled in the configuration file, exiting...")
		netdataapi.New(a.Out).DISABLE()
		return nil
	}

	enabledModules := a.loadEnabledModules(cfg)
	if len(enabledModules) == 0 {
		a.Info("no modules to run")
		netdataapi.New(a.Out).DISABLE()
		return nil
	}

	discCfg := a.buildDiscoveryConf(enabledModules)

	var runJob []string
	if a.RunModule != "" && a.RunModule != "all" {
		runJob = a.RunJob
	}
	process, err := composition.NewProcess(composition.Config{
		Input: a.In, Output: a.Out,
		PluginName: a.Name, Modules: enabledModules,
		Defaults:              discCfg.Defaults,
		DiscoveryBuildContext: discCfg.BuildContext,
		DiscoveryProviders:    discCfg.Providers,
		RunJob:                runJob,
		AutoEnable:            a.runModePolicy.AutoEnableDiscovered,
		InitialSecrets:        a.setupSecretStoreConfigs(),
		InitialVnodes:         a.setupVnodeRegistry(),
		Runtime:               a.setupRuntimeService(),
		KeepAlive:             !a.runModePolicy.IsTerminal,
		ShutdownTimeout:       a.ShutdownTimeout,
	})
	if err != nil {
		return err
	}
	a.processMu.Lock()
	if a.process != nil {
		a.processMu.Unlock()
		return errors.New("agent: process already constructed")
	}
	a.process = process
	a.processMu.Unlock()
	a.markProcessReady()
	return process.Run(ctx)
}

func (a *Agent) serviceDiscoveryEnabled() bool {
	if a == nil {
		return false
	}
	return !a.DisableServiceDiscovery && a.runModePolicy.EnableServiceDiscovery
}

func (a *Agent) setupRuntimeService() composition.RuntimeService {
	if a == nil || !a.runModePolicy.EnableRuntimeCharts {
		return nil
	}

	return runtimechartemit.New(
		a.Logger.With(slog.String("component", "runtime metrics service")),
	)
}

func (a *Agent) markProcessReady() {
	if a != nil {
		a.readyOnce.Do(func() { close(a.processReady) })
	}
}

func (a *Agent) awaitProcess(ctx context.Context) (*composition.Process, error) {
	if a == nil || ctx == nil {
		return nil, ErrNotRunning
	}
	select {
	case <-a.processReady:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	a.processMu.Lock()
	defer a.processMu.Unlock()
	if a.process == nil {
		return nil, ErrNotRunning
	}
	return a.process, nil
}
