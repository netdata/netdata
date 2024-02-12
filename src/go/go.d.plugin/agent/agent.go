// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/discovery"
	"github.com/netdata/go.d.plugin/agent/filelock"
	"github.com/netdata/go.d.plugin/agent/filestatus"
	"github.com/netdata/go.d.plugin/agent/functions"
	"github.com/netdata/go.d.plugin/agent/jobmgr"
	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/agent/netdataapi"
	"github.com/netdata/go.d.plugin/agent/safewriter"
	"github.com/netdata/go.d.plugin/agent/vnodes"
	"github.com/netdata/go.d.plugin/logger"
	"github.com/netdata/go.d.plugin/pkg/multipath"

	"github.com/mattn/go-isatty"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd())

// Config is an Agent configuration.
type Config struct {
	Name              string
	ConfDir           []string
	ModulesConfDir    []string
	ModulesSDConfPath []string
	VnodesConfDir     []string
	StateFile         string
	LockDir           string
	ModuleRegistry    module.Registry
	RunModule         string
	MinUpdateEvery    int
}

// Agent represents orchestrator.
type Agent struct {
	*logger.Logger

	Name              string
	ConfDir           multipath.MultiPath
	ModulesConfDir    multipath.MultiPath
	ModulesSDConfPath []string
	VnodesConfDir     multipath.MultiPath
	StateFile         string
	LockDir           string
	RunModule         string
	MinUpdateEvery    int
	ModuleRegistry    module.Registry
	Out               io.Writer

	api *netdataapi.API
}

// New creates a new Agent.
func New(cfg Config) *Agent {
	return &Agent{
		Logger: logger.New().With(
			slog.String("component", "agent"),
		),
		Name:              cfg.Name,
		ConfDir:           cfg.ConfDir,
		ModulesConfDir:    cfg.ModulesConfDir,
		ModulesSDConfPath: cfg.ModulesSDConfPath,
		VnodesConfDir:     cfg.VnodesConfDir,
		StateFile:         cfg.StateFile,
		LockDir:           cfg.LockDir,
		RunModule:         cfg.RunModule,
		MinUpdateEvery:    cfg.MinUpdateEvery,
		ModuleRegistry:    module.DefaultRegistry,
		Out:               safewriter.Stdout,
		api:               netdataapi.New(safewriter.Stdout),
	}
}

// Run starts the Agent.
func (a *Agent) Run() {
	go a.keepAlive()
	serve(a)
}

func serve(a *Agent) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup

	var exit bool
	var reload bool

	for {
		ctx, cancel := context.WithCancel(context.Background())
		ctx = context.WithValue(ctx, "reload", reload)

		wg.Add(1)
		go func() { defer wg.Done(); a.run(ctx) }()

		switch sig := <-ch; sig {
		case syscall.SIGHUP:
			a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
		default:
			a.Infof("received %s signal (%d). Terminating...", sig, sig)
			module.DontObsoleteCharts()
			exit = true
		}

		cancel()

		func() {
			timeout := time.Second * 10
			t := time.NewTimer(timeout)
			defer t.Stop()
			done := make(chan struct{})

			go func() { wg.Wait(); close(done) }()

			select {
			case <-t.C:
				a.Errorf("stopping all goroutines timed out after %s. Exiting...", timeout)
				os.Exit(0)
			case <-done:
			}
		}()

		if exit {
			os.Exit(0)
		}

		reload = true
		time.Sleep(time.Second)
	}
}

func (a *Agent) run(ctx context.Context) {
	a.Info("instance is started")
	defer func() { a.Info("instance is stopped") }()

	cfg := a.loadPluginConfig()
	a.Infof("using config: %s", cfg.String())

	if !cfg.Enabled {
		a.Info("plugin is disabled in the configuration file, exiting...")
		if isTerminal {
			os.Exit(0)
		}
		_ = a.api.DISABLE()
		return
	}

	enabledModules := a.loadEnabledModules(cfg)
	if len(enabledModules) == 0 {
		a.Info("no modules to run")
		if isTerminal {
			os.Exit(0)
		}
		_ = a.api.DISABLE()
		return
	}

	discCfg := a.buildDiscoveryConf(enabledModules)

	discoveryManager, err := discovery.NewManager(discCfg)
	if err != nil {
		a.Error(err)
		if isTerminal {
			os.Exit(0)
		}
		return
	}

	functionsManager := functions.NewManager()

	jobsManager := jobmgr.NewManager()
	jobsManager.PluginName = a.Name
	jobsManager.Out = a.Out
	jobsManager.Modules = enabledModules

	// TODO: API will be changed in https://github.com/netdata/netdata/pull/16702
	//if logger.Level.Enabled(slog.LevelDebug) {
	//	dyncfgDiscovery, _ := dyncfg.NewDiscovery(dyncfg.Config{
	//		Plugin:               a.Name,
	//		API:                  netdataapi.New(a.Out),
	//		Modules:              enabledModules,
	//		ModuleConfigDefaults: discCfg.Registry,
	//		Functions:            functionsManager,
	//	})
	//
	//	discoveryManager.Add(dyncfgDiscovery)
	//
	//	jobsManager.Dyncfg = dyncfgDiscovery
	//}

	if reg := a.setupVnodeRegistry(); reg == nil || reg.Len() == 0 {
		vnodes.Disabled = true
	} else {
		jobsManager.Vnodes = reg
	}

	if a.LockDir != "" {
		jobsManager.FileLock = filelock.New(a.LockDir)
	}

	var statusSaveManager *filestatus.Manager
	if !isTerminal && a.StateFile != "" {
		statusSaveManager = filestatus.NewManager(a.StateFile)
		jobsManager.StatusSaver = statusSaveManager
		if store, err := filestatus.LoadStore(a.StateFile); err != nil {
			a.Warningf("couldn't load state file: %v", err)
		} else {
			jobsManager.StatusStore = store
		}
	}

	in := make(chan []*confgroup.Group)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); functionsManager.Run(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); jobsManager.Run(ctx, in) }()

	wg.Add(1)
	go func() { defer wg.Done(); discoveryManager.Run(ctx, in) }()

	if statusSaveManager != nil {
		wg.Add(1)
		go func() { defer wg.Done(); statusSaveManager.Run(ctx) }()
	}

	wg.Wait()
	<-ctx.Done()
}

func (a *Agent) keepAlive() {
	if isTerminal {
		return
	}

	tk := time.NewTicker(time.Second)
	defer tk.Stop()

	var n int
	for range tk.C {
		if err := a.api.EMPTYLINE(); err != nil {
			a.Infof("keepAlive: %v", err)
			n++
		} else {
			n = 0
		}
		if n == 3 {
			a.Info("too many keepAlive errors. Terminating...")
			os.Exit(0)
		}
	}
}
