// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/cmd/internal/agenthost"
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/hostinfo"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
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
	_, _ = maxprocs.Set(maxprocs.Logger(func(s string, args ...interface{}) {}))

	opts := parseCLI()

	if opts.Version {
		fmt.Printf("%s.plugin, version: %s\n", executable.Name, buildinfo.Version)
		return
	}

	pluginconfig.MustInit(opts)

	if opts.Function != "" {
		os.Exit(runFunctionCLI(opts))
	}

	if lvl := pluginconfig.EnvLogLevel(); lvl != "" {
		logger.Level.SetByName(lvl)
	}
	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	isTerminal := terminal.IsTerminal()
	isInsideK8s := hostinfo.IsInsideK8sCluster()
	moduleRegistry := moduleRegistryWithSystemdPolicy(collectorapi.DefaultRegistry, hostinfo.SystemdVersion)

	a := agent.New(agent.Config{
		Name:                      executable.Name,
		PluginConfigDir:           pluginconfig.ConfigDir(),
		CollectorsConfigDir:       pluginconfig.CollectorsDir(),
		ServiceDiscoveryConfigDir: pluginconfig.ServiceDiscoveryDir(),
		CollectorsConfigWatchPath: pluginconfig.CollectorsConfigWatchPaths(),
		VarLibDir:                 pluginconfig.VarLibDir(),
		ModuleRegistry:            moduleRegistry,
		IsInsideK8s:               isInsideK8s,
		RunModePolicy: policy.RunModePolicy{
			IsTerminal:               isTerminal,
			AutoEnableDiscovered:     isTerminal,
			UseFileStatusPersistence: !isTerminal,
		},
		DiscoveryProviders: []discovery.ProviderFactory{
			discoveryproviders.File(),
			discoveryproviders.Dummy(),
			discoveryproviders.SD(sdext.Registry(!isInsideK8s)),
		},
		RunModule:      opts.Module,
		RunJob:         opts.Job,
		MinUpdateEvery: opts.UpdateEvery,
		DumpSummary:    opts.DumpSummary,
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

func runFunctionCLI(opts *cli.Option) int {
	functionName := strings.TrimSpace(opts.Function)
	if functionName == "" {
		writeFunctionError(400, "missing function name (expected module:method)")
		return 1
	}

	moduleName, methodID, err := functions.SplitFunctionName(functionName)
	if err != nil {
		writeFunctionError(400, "%v", err)
		return 1
	}

	creator, ok := collectorapi.DefaultRegistry.Lookup(moduleName)
	if !ok {
		writeFunctionError(404, "unknown module '%s'", moduleName)
		return 1
	}
	if creator.Methods == nil {
		writeFunctionError(404, "module '%s' does not expose functions", moduleName)
		return 1
	}
	if methodID == "" {
		writeFunctionError(400, "missing method name in function '%s'", functionName)
		return 1
	}

	payloadBytes, payloadTimeout, err := readFunctionPayload(opts.FunctionPayload)
	if err != nil {
		writeFunctionError(400, "%v", err)
		return 1
	}

	timeout, err := resolveFunctionTimeout(opts.FunctionTimeout, payloadTimeout)
	if err != nil {
		writeFunctionError(400, "%v", err)
		return 1
	}

	reg := confgroup.Registry{}
	reg.Register(moduleName, confgroup.Default{
		MinUpdateEvery:     opts.UpdateEvery,
		UpdateEvery:        creator.UpdateEvery,
		AutoDetectionRetry: creator.AutoDetectionRetry,
		Priority:           creator.Priority,
	})

	groups, err := loadConfigGroups(moduleName, reg, pluginconfig.CollectorsDir())
	if err != nil {
		writeFunctionError(500, "%v", err)
		return 1
	}
	if len(groups) == 0 {
		writeFunctionError(404, "no configs found for module '%s'", moduleName)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobMgr := jobmgr.New(jobmgr.Config{
		PluginName: executable.Name,
		Out:        io.Discard,
		RunModePolicy: policy.RunModePolicy{
			IsTerminal:               false,
			AutoEnableDiscovered:     true,
			UseFileStatusPersistence: true,
		},
		VarLibDir:      pluginconfig.VarLibDir(),
		Modules:        collectorapi.Registry{moduleName: creator},
		ConfigDefaults: reg,
		FnReg:          functions.NewManager(),
		FunctionJSONWriter: func(payload []byte, _ int) {
			_, _ = os.Stdout.Write(payload)
			_, _ = os.Stdout.Write([]byte("\n"))
		},
	})
	jobMgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(io.Discard)))

	in := make(chan []*confgroup.Group, 1)
	go jobMgr.Run(ctx, in)

	startCtx, startCancel := context.WithTimeout(ctx, time.Second*10)
	defer startCancel()
	if ok := jobMgr.WaitStarted(startCtx); !ok {
		writeFunctionError(503, "job manager failed to start")
		return 1
	}

	in <- groups

	if err := waitForJobs(startCtx, jobMgr, moduleName); err != nil {
		writeFunctionError(503, "%v", err)
		return 1
	}

	fn := functions.Function{
		Name:        functionName,
		Args:        opts.FunctionArgs,
		Payload:     payloadBytes,
		Timeout:     timeout,
		ContentType: "application/json",
	}
	jobMgr.ExecuteFunction(functionName, fn)

	return 0
}

func readFunctionPayload(raw string) ([]byte, time.Duration, error) {
	if raw == "" {
		return nil, 0, nil
	}

	var data []byte
	var err error
	if strings.HasPrefix(raw, "@") {
		data, err = os.ReadFile(strings.TrimPrefix(raw, "@"))
	} else {
		data = []byte(raw)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read payload: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, 0, fmt.Errorf("parse payload JSON: %w", err)
	}

	timeoutMs, ok, err := parsePayloadTimeout(payload)
	if err != nil {
		return nil, 0, err
	}
	if ok {
		return data, time.Duration(timeoutMs) * time.Millisecond, nil
	}
	return data, 0, nil
}

func parsePayloadTimeout(payload map[string]any) (int64, bool, error) {
	if payload == nil {
		return 0, false, nil
	}
	raw, ok := payload["timeout"]
	if !ok {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), true, nil
	case int:
		return int64(v), true, nil
	case int64:
		return v, true, nil
	case string:
		if v == "" {
			return 0, false, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("invalid payload timeout '%s'", v)
		}
		return n, true, nil
	default:
		return 0, false, fmt.Errorf("invalid payload timeout type %T", raw)
	}
}

func resolveFunctionTimeout(flagValue string, payloadTimeout time.Duration) (time.Duration, error) {
	if flagValue != "" {
		d, err := time.ParseDuration(flagValue)
		if err == nil {
			return d, nil
		}
		secs, err2 := strconv.ParseInt(flagValue, 10, 64)
		if err2 != nil {
			return 0, fmt.Errorf("invalid function-timeout '%s'", flagValue)
		}
		return time.Duration(secs) * time.Second, nil
	}
	if payloadTimeout > 0 {
		return payloadTimeout, nil
	}
	return time.Minute, nil
}

func loadConfigGroups(moduleName string, reg confgroup.Registry, collectors multipath.MultiPath) ([]*confgroup.Group, error) {
	if path, err := collectors.Find(moduleName + ".conf"); err == nil && path != "" {
		reader := file.NewReader(reg, []string{path})
		return runDiscoverer(reader)
	}

	disc, err := dummy.NewDiscovery(dummy.Config{
		Registry: reg,
		Names:    []string{moduleName},
	})
	if err != nil {
		return nil, err
	}
	return runDiscoverer(disc)
}

type discoverer interface {
	Run(ctx context.Context, in chan<- []*confgroup.Group)
}

func runDiscoverer(d discoverer) ([]*confgroup.Group, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	ch := make(chan []*confgroup.Group, 1)
	go d.Run(ctx, ch)

	select {
	case groups, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("discoverer returned no groups")
		}
		return groups, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("discoverer timeout")
	}
}

func waitForJobs(ctx context.Context, mgr *jobmgr.Manager, moduleName string) error {
	for {
		if len(mgr.GetJobNames(moduleName)) > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("no jobs started for module '%s'", moduleName)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func writeFunctionError(status int, format string, args ...any) {
	resp := map[string]any{
		"status":       status,
		"errorMessage": fmt.Sprintf(format, args...),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "{\"status\":%d,\"errorMessage\":\"%s\"}\n", status, "failed to encode error response")
		return
	}
	_, _ = os.Stdout.Write(data)
	_, _ = os.Stdout.Write([]byte("\n"))
}
