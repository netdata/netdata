// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"

	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	// Register IBM ecosystem collectors
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/as400"         // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/db2"           // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/mq_pcf"        // Legacy MQ collector
	// Note: New framework-based MQ module is registered below
	// _ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/websphere_jmx" // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/websphere_mp"  // Pure Go
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/collector/websphere_pmi" // Pure Go
	
	// New framework example
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/example" // Framework demo
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq"      // MQ monitoring with PCF protocol
)

const pluginName = "ibm.d.plugin"

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
		fmt.Printf("ibm.d.plugin, version: %s\n", buildinfo.Version)
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
		Name:                      cfg.name,
		PluginConfigDir:           cfg.pluginDir,
		CollectorsConfigDir:       cfg.collectorsDir,
		ServiceDiscoveryConfigDir: cfg.serviceDiscoveryDir,
		CollectorsConfigWatchPath: cfg.collectorsWatchPath,
		VarLibDir:                 cfg.varLibDir,
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
