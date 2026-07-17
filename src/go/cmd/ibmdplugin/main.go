// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/jessevdk/go-flags"
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
	// Register IBM ecosystem collectors
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/as400"         // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2"           // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq"            // MQ monitoring with PCF protocol
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/jmx" // Requires CGO
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/mp"  // Pure Go
	_ "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/pmi" // Requires CGO
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

	opts, err := parseCLI()
	if err != nil {
		if cli.IsHelp(err) {
			os.Exit(0)
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

	if lvl := pluginconfig.EnvLogLevel(); lvl != "" {
		logger.Level.SetByName(lvl)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	isTerminal := terminal.IsTerminal()

	a := agent.New(agent.Config{
		Name:                executable.Name,
		PluginConfigDir:     pluginconfig.ConfigDir(),
		CollectorsConfigDir: pluginconfig.CollectorsDir(),
		VarLibDir:           pluginconfig.VarLibDir(),
		ModuleRegistry:      collectorapi.DefaultRegistry,
		IsInsideK8s:         hostinfo.IsInsideK8sCluster(),
		RunModePolicy:       policy.Agent(isTerminal),
		DiscoveryProviders: []discovery.ProviderFactory{
			discoveryproviders.File(),
			discoveryproviders.Dummy(),
		},
		RunModule:               opts.Module,
		RunJob:                  opts.Job,
		MinUpdateEvery:          opts.UpdateEvery,
		DisableServiceDiscovery: true,
	})

	a.Debugf("plugin: name=%s, %s", a.Name, buildinfo.Info())
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	proxyCfg := httpproxy.FromEnvironment()
	a.Infof("env HTTP_PROXY '%s', HTTPS_PROXY '%s'", proxyCfg.HTTPProxy, proxyCfg.HTTPSProxy)

	a.Infof("directories → config: %s | collectors: %s | sd: %s | varlib: %s",
		a.ConfigDir, a.CollectorsConfDir, a.ServiceDiscoveryConfigDir, a.VarLibDir)

	agenthost.Run(a)
}

type options struct {
	cli.Option
}

func parseCLI() (*options, error) {
	opt := &options{
		Option: cli.Option{
			UpdateEvery: 1,
		},
	}

	parser := flags.NewParser(opt, flags.Default)
	parser.Name = executable.Name
	parser.Usage = "[OPTIONS] [update every]"

	rest, err := parser.ParseArgs(os.Args)
	if err != nil {
		return nil, err
	}

	if len(rest) > 1 {
		if opt.UpdateEvery, err = strconv.Atoi(rest[1]); err != nil {
			return nil, err
		}
	}

	return opt, nil
}
