// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/otelcolpoc"
	"go.uber.org/automaxprocs/maxprocs"
)

func init() {
	executable.Name = "otel-orchestrator"
	if strings.HasPrefix(os.Getenv("TZ"), ":") {
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
	if level := pluginconfig.EnvLogLevel(); level != "" {
		logger.Level.SetByName(level)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	endpoint := os.Getenv("NETDATA_OTEL_POC_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:4317"
	}
	state := os.Getenv("NETDATA_OTEL_POC_STATE_DIR")
	if state == "" {
		state = filepath.Join(pluginconfig.VarLibDir(), "otel-orchestrator")
	}
	registry := otelcolpoc.Registry(filepath.Join(executable.Directory, "otel-worker"), state, endpoint)
	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            registry,
		RunModePolicy:             policy.Agent(terminal.IsTerminal()),
		DiscoveryProviders:        []discovery.ProviderFactory{discoveryproviders.File(), manualDiscovery()},
		DisableServiceDiscovery:   true,
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
	})
	result := agenthost.Run(a)
	if result.Err != nil {
		a.Errorf("plugin exiting: %v", result.Err)
		os.Exit(1)
	}
	if result.ExitRequired {
		os.Exit(0)
	}
}
