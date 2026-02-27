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
	"strconv"
	"strings"
	"time"

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

	if opts.MetricsAuditDataDir != "" && opts.MetricsAuditDuration == "" {
		opts.MetricsAuditDuration = "10m"
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

	// Parse metrics-audit duration if provided.
	var auditDuration time.Duration
	if opts.MetricsAuditDuration != "" {
		var err error
		auditDuration, err = time.ParseDuration(opts.MetricsAuditDuration)
		if err != nil {
			logger.Errorf("error: invalid --metrics-audit duration '%s': %v", opts.MetricsAuditDuration, err)
			os.Exit(1)
		}
		if auditDuration <= 0 {
			logger.Errorf("error: invalid --metrics-audit duration '%s': duration must be > 0", opts.MetricsAuditDuration)
			os.Exit(1)
		}
	}

	if opts.MetricsAuditDataDir != "" && auditDuration <= 0 {
		logger.Errorf("error: --metrics-audit-data requires positive --metrics-audit duration")
		os.Exit(1)
	}

	metricsAuditDataDir := ""
	if opts.MetricsAuditDataDir != "" {
		var err error
		metricsAuditDataDir, err = prepareMetricsAuditDataDir(opts.MetricsAuditDataDir, pluginconfig.VarLibDir())
		if err != nil {
			logger.Errorf("error preparing --metrics-audit-data directory: %v", err)
			os.Exit(1)
		}
	}

	a := agent.New(agent.Config{
		Name:                executable.Name,
		PluginConfigDir:     pluginconfig.ConfigDir(),
		CollectorsConfigDir: pluginconfig.CollectorsDir(),
		VarLibDir:           pluginconfig.VarLibDir(),
		ModuleRegistry:      collectorapi.DefaultRegistry,
		IsInsideK8s:         hostinfo.IsInsideK8sCluster(),
		RunModePolicy: policy.RunModePolicy{
			IsTerminal:               isTerminal,
			AutoEnableDiscovered:     isTerminal,
			UseFileStatusPersistence: !isTerminal,
		},
		DiscoveryProviders: []discovery.ProviderFactory{
			discoveryproviders.File(),
			discoveryproviders.Dummy(),
		},
		RunModule:               opts.Module,
		RunJob:                  opts.Job,
		MinUpdateEvery:          opts.UpdateEvery,
		AuditDuration:           auditDuration,
		AuditSummary:            opts.MetricsAuditSummary,
		AuditDataDir:            metricsAuditDataDir,
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
	MetricsAuditDuration string `long:"metrics-audit" description:"run metrics-audit mode for specified duration (e.g. 30s, 5m) and analyze metric structure"`
	MetricsAuditSummary  bool   `long:"metrics-audit-summary" description:"show consolidated metrics-audit summary across all jobs"`
	MetricsAuditDataDir  string `long:"metrics-audit-data" description:"write structured metrics-audit artifacts for the selected module to the given directory"`
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

func prepareMetricsAuditDataDir(path string, varLibDir string) (string, error) {
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
		return "", fmt.Errorf("refusing to use unsafe metrics-audit-data directory '%s'", absolute)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", err
	}
	runDir := filepath.Join(absolute, fmt.Sprintf("run-%s", time.Now().UTC().Format("20060102-150405.000000000")))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	return runDir, nil
}
