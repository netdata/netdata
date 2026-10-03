// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	_ "github.com/netdata/netdata/go/plugins/plugin/ebpf/collector/cachestat"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func init() { executable.Name = "ebpf-poc" }

func main() {
	opts, err := cli.Parse(os.Args)
	if err != nil {
		if cli.IsHelp(err) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if opts.Version {
		fmt.Println("ebpf-poc.plugin (experimental shared-framework port)")
		return
	}
	pluginconfig.MustInit(pluginconfig.InitInput{ConfDir: opts.ConfDir, WatchPath: opts.WatchPath})
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            collectorapi.DefaultRegistry,
		RunModePolicy:             policy.Agent(terminal.IsTerminal()),
		DiscoveryProviders:        []discovery.ProviderFactory{discoveryproviders.File(), discoveryproviders.Dummy()},
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
		DisableServiceDiscovery:   true,
	})
	if err := agenthost.Run(a); err != nil {
		a.Errorf("POC agent failed: %v", err)
		os.Exit(1)
	}
}
