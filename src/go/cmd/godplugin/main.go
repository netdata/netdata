// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pluginconfig"

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

	pluginconfig.MustInit(opts)

	if lvl := pluginconfig.EnvLogLevel(); lvl != "" {
		logger.Level.SetByName(lvl)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}

	// Parse dump duration if provided
	var dumpMode time.Duration
	if opts.DumpMode != "" {
		var err error
		dumpMode, err = time.ParseDuration(opts.DumpMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid dump duration '%s': %v\n", opts.DumpMode, err)
			os.Exit(1)
		}
	}

	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		ServiceDiscoveryConfigDir: pluginconfig.ServiceDiscoveryDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		RunModule:                 opts.Module,
		RunJob:                    opts.Job,
		MinUpdateEvery:            opts.UpdateEvery,
		DumpMode:                  dumpMode,
		DumpSummary:               opts.DumpSummary,
	})

	a.Debugf("plugin: name=%s, version=%s", a.Name, buildinfo.Version)
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	proxyCfg := httpproxy.FromEnvironment()
	a.Infof("env HTTP_PROXY '%s', HTTPS_PROXY '%s'", proxyCfg.HTTPProxy, proxyCfg.HTTPSProxy)

	a.Infof("directories → config: %s | collectors: %s | sd: %s | varlib: %s",
		a.ConfigDir, a.CollectorsConfDir, a.ServiceDiscoveryConfigDir, a.VarLibDir)

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
