// SPDX-License-Identifier: GPL-3.0-or-later
// +build ignore

// This is the new main implementation using agent orchestration.
// To use this, rename main.go to main_legacy.go and this to main.go

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"
)

func init() {
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}

func mainNew() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(string, ...any) {}))

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

	registry := NewEbpfRegistry()
	var services []interface{}
	if !isTerminal {
		// In non-terminal mode, could add publisher or other services here
	}

	runModePolicy := policy.Agent(isTerminal)

	a := agent.New(agent.Config{
		Name:                      "ebpfgo.plugin",
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		ServiceDiscoveryConfigDir: pluginconfig.ServiceDiscoveryDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            registry,
		// Services:                  services,
		IsInsideK8s:   isInsideK8s,
		RunModePolicy: runModePolicy,
		RunModule:     opts.Module,
		RunJob:        opts.Job,
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

	if err := agenthost.Run(a); err != nil {
		a.Errorf("plugin exiting after Agent failure: %v", err)
		os.Exit(1)
	}
}
