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
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/mattn/go-isatty"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd())

// Config is an Agent configuration.
type Config struct {
	Name                      string
	PluginConfigDir           []string
	CollectorsConfigDir       []string
	CollectorsConfigWatchPath []string
	ServiceDiscoveryConfigDir []string
	VarLibDir                 string
	ModuleRegistry            module.Registry
	RunModule                 string
	MinUpdateEvery            int
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

	RunModule      string
	MinUpdateEvery int

	ModuleRegistry module.Registry
	Out            io.Writer

	api *netdataapi.API

	quitCh chan struct{}
}

// New creates a new Agent.
func New(cfg Config) *Agent {
	return &Agent{
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
		MinUpdateEvery:            cfg.MinUpdateEvery,
		ModuleRegistry:            module.DefaultRegistry,
		Out:                       safewriter.Stdout,
		api:                       netdataapi.New(safewriter.Stdout),
		quitCh:                    make(chan struct{}, 1),
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
	signal.Ignore(syscall.SIGPIPE)

	var wg sync.WaitGroup

	var exit bool

	for {
		module.ObsoleteCharts(true)

		ctx, cancel := context.WithCancel(context.Background())

		wg.Add(1)
		go func() { defer wg.Done(); a.run(ctx) }()

		select {
		case sig := <-ch:
			switch sig {
			case syscall.SIGHUP:
				a.Infof("received %s signal (%d). Restarting running instance", sig, sig)
			default:
				a.Infof("received %s signal (%d). Terminating...", sig, sig)
				exit = true
			}
		case <-a.quitCh:
			a.Infof("received QUIT command. Terminating...")
			exit = true
		}

		if exit {
			module.ObsoleteCharts(false)
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
		a.api.DISABLE()
		return
	}

	enabledModules := a.loadEnabledModules(cfg)
	if len(enabledModules) == 0 {
		a.Info("no modules to run")
		if isTerminal {
			os.Exit(0)
		}
		a.api.DISABLE()
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
	jobMgr.VarLibDir = a.VarLibDir
	jobMgr.Modules = enabledModules
	jobMgr.ConfigDefaults = discCfg.Registry
	jobMgr.FnReg = fnMgr

	if reg := a.setupVnodeRegistry(); len(reg) > 0 {
		jobMgr.Vnodes = reg
	}

	in := make(chan []*confgroup.Group)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); fnMgr.Run(ctx, a.quitCh) }()

	wg.Add(1)
	go func() { defer wg.Done(); jobMgr.Run(ctx, in) }()

	wg.Add(1)
	go func() { defer wg.Done(); discMgr.Run(ctx, in) }()

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
			n++
		} else {
			n = 0
		}
		if n >= 30 {
			a.Info("too many keepAlive errors. Terminating...")
			os.Exit(0)
		}
	}
}
