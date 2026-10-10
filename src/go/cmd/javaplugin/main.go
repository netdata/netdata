// SPDX-License-Identifier: GPL-3.0-or-later

// javaplugin monitors local JVMs using the Netdata Go collector framework.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/java/collector"
)

func main() {
	executable.Name = "java"
	pluginconfig.MustInit(pluginconfig.InitInput{})
	runtime := collector.RuntimeConfig{StateDir: filepath.Join(pluginconfig.VarLibDir(), "java")}

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
