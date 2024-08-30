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

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/filestatus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/multipath"

	"github.com/mattn/go-isatty"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd())

// Config is an Agent configuration.
type Config struct {
	Name                 string
	ConfDir              []string
	ModulesConfDir       []string
	ModulesConfSDDir     []string
	ModulesConfWatchPath []string
	VnodesConfDir        []string
	StateFile            string
	LockDir              string
	ModuleRegistry       module.Registry
	RunModule            string
	MinUpdateEvery       int
}

// Agent represents orchestrator.
type Agent struct {
	*logger.Logger

	Name              string
	ConfDir           multipath.MultiPath
	ModulesConfDir    multipath.MultiPath
	ModulesConfSDDir  multipath.MultiPath
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
		ModulesConfSDDir:  cfg.ModulesConfSDDir,
		ModulesSDConfPath: cfg.ModulesConfWatchPath,
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

	for {
		ctx, cancel := context.WithCancel(context.Background())

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

	discMgr, err := discovery.NewManager(discCfg)
	if err != nil {
		a.Error(err)
		if isTerminal {
			os.Exit(0)
		}
		return
	}

	fnMgr := functions.NewManager()

	jobMgr := jobmgr.New()
	jobMgr.PluginName = a.Name
	jobMgr.Out = a.Out
	jobMgr.Modules = enabledModules
	jobMgr.ConfigDefaults = discCfg.Registry
	jobMgr.FnReg = fnMgr

	if reg := a.setupVnodeRegistry(); reg != nil && reg.Len() > 0 {
		jobMgr.Vnodes = reg
	}

	if a.LockDir != "" {
		jobMgr.FileLock = filelock.New(a.LockDir)
	}

	var fsMgr *filestatus.Manager
	if !isTerminal && a.StateFile != "" {
		fsMgr = filestatus.NewManager(a.StateFile)
		jobMgr.FileStatus = fsMgr
		if store, err := filestatus.LoadStore(a.StateFile); err != nil {
			a.Warningf("couldn't load state file: %v", err)
		} else {
			jobMgr.FileStatusStore = store
		}
	}

	in := make(chan []*confgroup.Group)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); fnMgr.Run(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); jobMgr.Run(ctx, in) }()

	wg.Add(1)
	go func() { defer wg.Done(); discMgr.Run(ctx, in) }()

	if fsMgr != nil {
		wg.Add(1)
		go func() { defer wg.Done(); fsMgr.Run(ctx) }()
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
