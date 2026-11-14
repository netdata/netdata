// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/cmd/internal/secretproviders"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"go.uber.org/automaxprocs/maxprocs"
)

func init() {
	executable.Name = "dem"
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}
func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(string, ...any) {}))
	runtimeContext := context.Background()
	stopRuntimeSignals := func() {}
	if len(os.Args) > 1 && os.Args[1] == "runtime" {
		runtimeContext, stopRuntimeSignals = signal.NotifyContext(runtimeContext, os.Interrupt, syscall.SIGTERM)
	}
	handled, runtimeError := runRuntimeCommandWithInput(runtimeContext, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stopRuntimeSignals()
	if handled {
		if err := runtimeError; err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	opts, err := cli.Parse(os.Args)
	if err != nil {
		if cli.IsHelp(err) {
			return
		}
		os.Exit(1)
	}
	if opts.Version {
		fmt.Printf("%s.plugin, version: %s\n", executable.Name, buildinfo.Version)
		return
	}
	pluginconfig.MustInit(pluginconfig.InitInput{
		ConfDir:   opts.ConfDir,
		WatchPath: opts.WatchPath,
	})
	if lvl := pluginconfig.EnvLogLevel(); lvl != "" {
		logger.Level.SetByName(lvl)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	isTerminal := terminal.IsTerminal()
	debug := isTerminal || opts.Debug
	cfg, err := dem.LoadConfig(pluginconfig.ConfigDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "initializing DEM config: %v\n", err)
		os.Exit(1)
	}
	secrets, err := secretproviders.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "initializing secrets: %v\n", err)
		os.Exit(1)
	}
	process, err := dem.Open(dem.Options{
		Config:     cfg,
		LoadConfig: func() (dem.Config, error) { return dem.LoadConfig(pluginconfig.ConfigDir()) },
		VarLibDir:  pluginconfig.VarLibDir(),
		Private:    debug,
		GeoIPPaths: geoip.AgentPaths(pluginconfig.CacheDir(), pluginconfig.StockDataDir()),
		Warnings:   os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            process.Collectors,
		ProcessFunctions:          process.Functions,
		Services:                  process.Services,
		Secrets:                   secrets,
		IsInsideK8s:               hostinfo.IsInsideK8sCluster(),
		RunModePolicy:             policy.Agent(isTerminal),
		DiscoveryProviders:        []discovery.ProviderFactory{discoveryproviders.File(), discoveryproviders.Dummy()},
		DisableServiceDiscovery:   true,
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
	})
	a.Infof("plugin: name=%s, %s", a.Name, buildinfo.Info())
	completed := make(chan agenthost.Result, 1)
	go func() { completed <- agenthost.Run(a) }()
	var result agenthost.Result
	select {
	case result = <-completed:
	case err := <-process.Fatal():
		a.Errorf("plugin exiting with retained unverified synthetic work: %v", err)
		os.Exit(1)
	}
	// A completed Agent and a simultaneous poison must still fail-stop.
	select {
	case err := <-process.Fatal():
		a.Errorf("unverified synthetic completion: %v", err)
		os.Exit(1)
	default:
	}
	// An error or forced recovery can leave active consumers. Process exit owns
	// their resources; only a joined, successful shutdown may close shared state.
	if result.Err != nil {
		a.Errorf("plugin exiting after Agent failure: %v", result.Err)
		os.Exit(1)
	}
	if result.ExitRequired {
		os.Exit(0)
	}
	if err := process.Close(); err != nil {
		a.Errorf("%v", err)
		os.Exit(1)
	}
}
