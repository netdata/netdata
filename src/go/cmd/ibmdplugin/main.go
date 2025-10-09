// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq
// +build cgo,ibm_mq

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"

	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

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

	opts := parseCLI()

	if opts.Version {
		fmt.Printf("ibm.d.plugin, version: %s\n", buildinfo.Version)
		return
	}

	if opts.DumpDataDir != "" && opts.DumpMode == "" {
		opts.DumpMode = "10m"
	}

	env := newEnvConfig()
	cfg := newConfig(opts, env)

	dumpDataDir := ""
	if opts.DumpDataDir != "" {
		var err error
		dumpDataDir, err = prepareDumpDataDir(opts.DumpDataDir, cfg.varLibDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error preparing dump-data directory: %v\n", err)
			os.Exit(1)
		}
	}

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
		DumpDataDir:               dumpDataDir,
		DynamicConfigPrefix:       "ibm.d:collector:",
		DisableServiceDiscovery:   true,
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

func prepareDumpDataDir(path string, varLibDir string) (string, error) {
	if path == "" {
		return "", nil
	}
	clean := strings.TrimSpace(path)
	if !filepath.IsAbs(clean) && varLibDir != "" {
		clean = filepath.Join(varLibDir, clean)
	}
	absolute, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	if absolute == "" || absolute == "/" {
		return "", fmt.Errorf("refusing to use unsafe dump-data directory '%s'", absolute)
	}
	if err := os.RemoveAll(absolute); err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", err
	}
	return absolute, nil
}
