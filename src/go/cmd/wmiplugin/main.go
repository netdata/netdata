// SPDX-License-Identifier: GPL-3.0-or-later

// wmi.plugin collects metrics from explicitly configured remote Windows hosts.
package main

import (
	"fmt"
	"log/slog"
	"os"
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
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	_ "github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows"
	"go.uber.org/automaxprocs/maxprocs"
)

func main() {
	executable.Name = "wmi"
	if strings.HasPrefix(os.Getenv("TZ"), ":") {
		_ = os.Unsetenv("TZ")
	}
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

	secrets, err := secretproviders.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "initializing secrets: %v\n", err)
		os.Exit(1)
	}

	a := agent.New(agent.Config{
		Name:                      executable.Name,
		Secrets:                   secrets,
		ModuleRegistry:            collectorapi.DefaultRegistry,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		RunModePolicy:             policy.Agent(terminal.IsTerminal()),
		IsInsideK8s:               hostinfo.IsInsideK8sCluster(),
		DiscoveryProviders:        []discovery.ProviderFactory{discoveryproviders.File()},
		DisableServiceDiscovery:   true,
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
	})
	a.Infof("plugin: name=%s, %s", a.Name, buildinfo.Info())
	result := agenthost.Run(a)
	if result.Err != nil {
		a.Errorf("plugin exiting after Agent failure: %v", result.Err)
		os.Exit(1)
	}
	if result.ExitRequired {
		os.Exit(0)
	}
}
