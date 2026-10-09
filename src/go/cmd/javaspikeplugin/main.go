// SPDX-License-Identifier: GPL-3.0-or-later

// javaspikeplugin is an experimental framework host, never installed by packaging.
package main

import (
	"fmt"
	"os"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/tools/java-monitoring-spike/collector"
)

func main() {
	executable.Name = "javaspike"
	pluginconfig.MustInit(pluginconfig.InitInput{})
	runtime := collector.RuntimeConfig{
		RunID:    os.Getenv("JAVASPIKE_RUN"),
		Scope:    os.Getenv("SCOUT_LAB_SCOPE"),
		Endpoint: os.Getenv("JAVASPIKE_ENDPOINT"),
		Listen:   "0.0.0.0:4318",
		Home:     "/lab",
		StateDir: "/state",
	}
	registry := collectorapi.Registry{}
	collector.Register(registry, runtime)
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            registry,
		RunModePolicy:             policy.Agent(false),
		DiscoveryProviders:        []discovery.ProviderFactory{discoveryproviders.File()},
		DisableServiceDiscovery:   true,
		MinUpdateEvery:            1,
	})
	result := agenthost.Run(a)
	if result.Err != nil {
		fmt.Fprintln(os.Stderr, result.Err)
		os.Exit(1)
	}
}
