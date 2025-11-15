// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	_ "github.com/netdata/netdata/go/plugins/plugin/scripts.d/modules/nagios"
)

func init() {
	executable.Name = "scripts.d"
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(string, ...interface{}) {}))

	opts := parseCLI()

	if opts.Version {
		fmt.Printf("%s.plugin, version: %s\n", executable.Name, buildinfo.Version)
		return
	}

	pluginconfig.MustInit(opts)

	watchPaths := pluginconfig.CollectorsConfigWatchPaths()
	if len(watchPaths) == 0 {
		for _, dir := range pluginconfig.CollectorsDir() {
			watchPaths = append(watchPaths, filepath.Join(dir, "*.conf"))
		}
	}

	if lvl := pluginconfig.EnvLogLevel(); lvl != "" {
		logger.Level.SetByName(lvl)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}

	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		ServiceDiscoveryConfigDir: nil,
		CollectorsConfigWatchPath: watchPaths,
		VarLibDir:                 pluginconfig.VarLibDir(),
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
		DumpSummary:               opts.DumpSummary,
		DisableServiceDiscovery:   true,
	})

	a.Debugf("plugin: name=%s, %s", a.Name, buildinfo.Info())
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	proxyCfg := httpproxy.FromEnvironment()
	a.Infof("env proxy settings: HTTP_PROXY set=%t, HTTPS_PROXY set=%t", proxyCfg.HTTPProxy != "", proxyCfg.HTTPSProxy != "")

	a.Infof("directories → config: %s | collectors: %s | varlib: %s",
		a.ConfigDir, a.CollectorsConfDir, a.VarLibDir)

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
