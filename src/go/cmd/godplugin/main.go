// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"

	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector"
)

func init() {
	// https://github.com/netdata/netdata/issues/8949#issuecomment-638294959
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(s string, args ...interface{}) {}))

	opts := parseCLI()

	if opts.Version {
		fmt.Printf("go.d.plugin, version: %s\n", buildinfo.Version)
		return
	}

	env := newEnvConfig()
	cfg := newConfig(opts, env)

	if env.logLevel != "" {
		logger.Level.SetByName(env.logLevel)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}

	a := agent.New(agent.Config{
		Name:                      cfg.name,
		PluginConfigDir:           cfg.pluginDir,
		CollectorsConfigDir:       cfg.collectorsDir,
		ServiceDiscoveryConfigDir: cfg.serviceDiscoveryDir,
		CollectorsConfigWatchPath: cfg.collectorsWatchPath,
		StateFile:                 cfg.stateFile,
		LockDir:                   cfg.lockDir,
		RunModule:                 opts.Module,
		MinUpdateEvery:            opts.UpdateEvery,
	})

	a.Debugf("plugin: name=%s, version=%s", a.Name, buildinfo.Version)
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	proxyCfg := httpproxy.FromEnvironment()
	a.Infof("env HTTP_PROXY '%s', HTTPS_PROXY '%s'", proxyCfg.HTTPProxy, proxyCfg.HTTPSProxy)

	a.Run()
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
