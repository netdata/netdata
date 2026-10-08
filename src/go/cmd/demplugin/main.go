// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
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
	path, err := historyPath(pluginconfig.VarLibDir(), debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initializing DEM history: %v\n", err)
		os.Exit(1)
	}
	history, err := demjournal.Open(context.Background(), path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening DEM history: %v\n", err)
		os.Exit(1)
	}
	// Apply the configured budget before any producer can append.
	if err := history.EnforceHistoryRetention(context.Background(), cfg.History.Days, cfg.History.MaxBytes); err != nil {
		_ = history.Close()
		fmt.Fprintf(os.Stderr, "initializing DEM history policy: %v\n", err)
		os.Exit(1)
	}
	artifactPath := ""
	if !debug {
		artifactPath = filepath.Join(pluginconfig.VarLibDir(), "dem", "artifacts")
	}
	captures, err := artifacts.Open(artifactPath)
	if err != nil {
		_ = history.Close()
		fmt.Fprintf(os.Stderr, "opening DEM artifacts: %v\n", err)
		os.Exit(1)
	}
	if _, err = captures.Enforce(context.Background(), cfg.Artifacts.Days, cfg.Artifacts.MaxBytes); err != nil {
		_ = captures.Close()
		_ = history.Close()
		fmt.Fprintf(os.Stderr, "initializing DEM artifact policy: %v\n", err)
		os.Exit(1)
	}
	cfg.Runtime.AssetsPath = filepath.Join(executable.Directory, "dem")
	executor := runner.New(cfg.Runtime, synthetichistory.NewStore(history), captures)
	components := dem.New(
		dem.Dependencies{
			History:        history,
			GeoIPPaths:     geoip.AgentPaths(pluginconfig.CacheDir(), pluginconfig.StockDataDir()),
			Artifacts:      captures,
			Executor:       executor,
			ConfigProvider: func() (dem.Config, error) { return dem.LoadConfig(pluginconfig.ConfigDir()) },
		},
		cfg,
	)
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            components.Collectors,
		ProcessFunctions:          components.Functions,
		Services:                  []composition.ProcessService{components.Retention},
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
	case err := <-executor.Fatal():
		a.Errorf("plugin exiting with retained unverified synthetic work: %v", err)
		os.Exit(1)
	}
	// A completed Agent and a simultaneous poison must still fail-stop.
	select {
	case err := <-executor.Fatal():
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
	if err := captures.Close(); err != nil {
		a.Errorf("closing DEM artifacts: %v", err)
		os.Exit(1)
	}
	if err := history.Close(); err != nil {
		a.Errorf("closing DEM history: %v", err)
		os.Exit(1)
	}
}
func historyPath(varLib string, debug bool) (string, error) {
	if debug {
		return "", nil
	}
	if strings.TrimSpace(varLib) == "" {
		return "", fmt.Errorf("NETDATA_LIB_DIR is required outside terminal/debug mode")
	}
	return filepath.Join(varLib, "dem", "journal"), nil
}
