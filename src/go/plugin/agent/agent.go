// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/runtimechartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/metricsaudit"
)

// Config is an Agent configuration.
type Config struct {
	Name            string
	PluginConfigDir []string

	CollectorsConfigDir       []string
	CollectorsConfigWatchPath []string
	ServiceDiscoveryConfigDir []string
	VarLibDir                 string

	ModuleRegistry collectorapi.Registry
	RunModule      string
	RunJob         []string
	MinUpdateEvery int

	DisableServiceDiscovery bool

	IsInsideK8s bool

	RunModePolicy policy.RunModePolicy

	DiscoveryProviders []discovery.ProviderFactory

	AuditDuration time.Duration
	AuditSummary  bool
	AuditDataDir  string
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
	RunJob         []string
	MinUpdateEvery int

	DisableServiceDiscovery bool

	IsInsideK8s bool

	runModePolicy policy.RunModePolicy

	DiscoveryProviders []discovery.ProviderFactory

	ModuleRegistry collectorapi.Registry
	Out            io.Writer

	api *netdataapi.API

	quitCh chan struct{}

	// Metrics-audit mode.
	auditDuration time.Duration
	auditSummary  bool
	auditAnalyzer *metricsaudit.Auditor

	auditDataDir string
	quitOnce     sync.Once
	auditOnce    sync.Once
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
		IsInsideK8s:               cfg.IsInsideK8s,
		runModePolicy:             cfg.RunModePolicy,
		ModuleRegistry:            cfg.ModuleRegistry,
		DiscoveryProviders:        cfg.DiscoveryProviders,
		Out:                       safewriter.Stdout,
		api:                       netdataapi.New(safewriter.Stdout),
		quitCh:                    make(chan struct{}, 1),
		auditDuration:             cfg.AuditDuration,
		auditSummary:              cfg.AuditSummary,
		DisableServiceDiscovery:   cfg.DisableServiceDiscovery,
	}

	if a.auditDuration > 0 {
		a.auditAnalyzer = metricsaudit.New()
		a.Infof("metrics-audit mode enabled: will run for %v and analyze metric structure", a.auditDuration)
		if a.auditSummary {
			a.Infof("metrics-audit summary enabled: will show consolidated summary across all jobs")
		}
	}

	if cfg.AuditDataDir != "" {
		a.auditDataDir = cfg.AuditDataDir
		if a.auditAnalyzer == nil {
			a.auditAnalyzer = metricsaudit.New()
		}
		a.auditAnalyzer.EnableDataCapture(cfg.AuditDataDir, a.signalAuditComplete)
		a.Infof("metrics-audit data directory: %s", cfg.AuditDataDir)
	}

	return a
}

// RunContext runs one agent instance lifecycle on the provided context.
func (a *Agent) RunContext(ctx context.Context) {
	a.run(ctx)
}

// IsTerminalMode reports whether run-mode policy is interactive terminal.
func (a *Agent) IsTerminalMode() bool {
	return a.runModePolicy.IsTerminal
}

// RunKeepAlive runs keepalive loop until context cancellation or too many failures.
func (a *Agent) RunKeepAlive(ctx context.Context) error {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()

	var n int
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tk.C:
			if err := a.api.EMPTYLINE(); err != nil {
				n++
			} else {
				n = 0
			}
			if n >= 30 {
				return fmt.Errorf("too many keepAlive errors")
			}
		}
	}
}

// QuitCh returns agent quit notifications (e.g., metrics-audit completion).
func (a *Agent) QuitCh() <-chan struct{} {
	return a.quitCh
}

// AuditDuration returns configured metrics-audit timer duration.
func (a *Agent) AuditDuration() time.Duration {
	return a.auditDuration
}

// FinalizeMetricsAudit prints metrics-audit analysis report once.
func (a *Agent) FinalizeMetricsAudit(reason string) {
	a.auditOnce.Do(func() {
		if a.auditAnalyzer == nil {
			return
		}
		if reason != "" {
			a.Infof("finalizing metrics audit (%s)", reason)
		}
		a.printMetricsAudit()
	})
}

func (a *Agent) run(ctx context.Context) {
	a.Info("instance is started")
	defer func() { a.Info("instance is stopped") }()

	cfg := a.loadPluginConfig()
	a.Infof("using config: %s", cfg.String())

	if !cfg.Enabled {
		a.Info("plugin is disabled in the configuration file, exiting...")
		a.api.DISABLE()
		return
	}

	enabledModules := a.loadEnabledModules(cfg)
	if len(enabledModules) == 0 {
		a.Info("no modules to run")
		a.api.DISABLE()
		return
	}

	fnMgr := functions.NewManager()

	discCfg := a.buildDiscoveryConf(enabledModules, fnMgr)

	discMgr, err := discovery.NewManager(discCfg)
	if err != nil {
		a.Error(err)
		return
	}

	runtimeSvc := runtimechartemit.New(a.Logger.With(slog.String("component", "runtime metrics service")))
	runtimeSvc.Start(a.Name, a.Out)
	defer runtimeSvc.Stop()

	var runJob []string
	if a.RunModule != "" && a.RunModule != "all" {
		runJob = a.RunJob
	}

	jobMgr := jobmgr.New(jobmgr.Config{
		PluginName:     a.Name,
		Out:            a.Out,
		RunModePolicy:  a.runModePolicy,
		Modules:        enabledModules,
		RunJob:         runJob,
		ConfigDefaults: discCfg.Registry,
		VarLibDir:      a.VarLibDir,
		FnReg:          fnMgr,
		Vnodes:         a.setupVnodeRegistry(),
		AuditMode:      a.auditDuration > 0,
		AuditAnalyzer:  a.auditAnalyzer,
		AuditDataDir:   a.auditDataDir,
		RuntimeService: runtimeSvc,
	})

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

func (a *Agent) printMetricsAudit() {
	if a.auditAnalyzer == nil {
		return
	}

	// Print the analysis report
	if a.auditSummary {
		a.auditAnalyzer.PrintSummary()
	} else {
		a.auditAnalyzer.PrintReport()
	}
}

func (a *Agent) signalAuditComplete() {
	a.quitOnce.Do(func() {
		a.Infof("metrics-audit data collection complete, shutting down")
		select {
		case a.quitCh <- struct{}{}:
		default:
		}
	})
}
