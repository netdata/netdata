// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext"
)

func init() {
	// https://github.com/netdata/netdata/issues/8949#issuecomment-638294959
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(s string, args ...any) {}))

	opts := parseCLI()

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
	isInsideK8s := hostinfo.IsInsideK8sCluster()
	moduleRegistry := moduleRegistryWithSystemdPolicy(collectorapi.DefaultRegistry, hostinfo.SystemdVersion)

	runModePolicy := policy.Agent(isTerminal)

	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		ServiceDiscoveryConfigDir: pluginconfig.ServiceDiscoveryDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            moduleRegistry,
		IsInsideK8s:               isInsideK8s,
		RunModePolicy:             runModePolicy,
		DiscoveryProviders: []discovery.ProviderFactory{
			discoveryproviders.File(),
			discoveryproviders.Dummy(),
			discoveryproviders.SD(sdext.Registry(!isInsideK8s)),
		},
		RunModule:      opts.Module,
		RunJob:         opts.Job,
		MinUpdateEvery: opts.UpdateEvery,
	})

	a.Infof("plugin: name=%s, %s", a.Name, buildinfo.Info())
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	proxyCfg := httpproxy.FromEnvironment()
	a.Infof("env HTTP_PROXY '%s', HTTPS_PROXY '%s'", proxyCfg.HTTPProxy, proxyCfg.HTTPSProxy)

	a.Infof("directories → config: %s | collectors: %s | sd: %s | varlib: %s",
		a.ConfigDir, a.CollectorsConfDir, a.ServiceDiscoveryConfigDir, a.VarLibDir)

	agenthost.Run(a)
}

func parseCLI() *cli.Option {
	opt, err := cli.Parse(os.Args)
	if err != nil {
		if cli.IsHelp(err) {
			os.Exit(0)
		}
		os.Exit(1)
	}

	return opt
}

func moduleRegistryWithSystemdPolicy(base collectorapi.Registry, systemdVersion int) collectorapi.Registry {
	registry := make(collectorapi.Registry, len(base))
	for name, creator := range base {
		if name == "logind" && systemdVersion == 239 {
			// Known issue: go.d/logind high CPU usage on Alma Linux8.
			// Keep policy in cmd wiring, not inside generic agent package.
			creator.Disabled = true
		}
		registry[name] = creator
	}
	return registry
}
