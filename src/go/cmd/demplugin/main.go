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
	"github.com/netdata/netdata/go/plugins/plugin/dem/healthgen"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
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
	history, err := store.Open(context.Background(), path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening DEM history: %v\n", err)
		os.Exit(1)
	}
	healthDir := filepath.Join(pluginconfig.VarLibDir(), "dem", "health.d")
	startupLog := logger.New()
	recovery := healthgen.NewRecovery(
		healthDir,
		healthgen.Options{
			Debug:   debug,
			OnError: func(err error) { startupLog.Warningf("DEM health recovery: %v", err) },
		},
	)
	if err := recovery.Prepare(); err != nil {
		startupLog.Warningf("DEM health file recovery: %v", err)
	}
	registry, retention := dem.NewRegistry(
		dem.Dependencies{
			History:        history,
			ConfigProvider: func() (dem.Config, error) { return dem.LoadConfig(pluginconfig.ConfigDir()) },
			HealthDir:      filepath.Join(pluginconfig.VarLibDir(), "dem", "health.d"),
			Debug:          debug,
		},
		cfg,
	)
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            registry,
		Services:                  []composition.ProcessService{retention, recovery},
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
	result := agenthost.Run(a)
	// An error or forced recovery can leave active consumers. Process exit owns
	// their resources; only a joined, successful shutdown may close shared state.
	if result.Err != nil {
		a.Errorf("plugin exiting after Agent failure: %v", result.Err)
		os.Exit(1)
	}
	if result.ExitRequired {
		os.Exit(0)
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
	return filepath.Join(varLib, "dem", "history.db"), nil
}
