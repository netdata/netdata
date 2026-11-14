// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Options are the resolved inputs of one plugin process.
type Options struct {
	Config Config
	// LoadConfig rereads dem.conf for the retention worker.
	LoadConfig func() (Config, error)
	// VarLibDir holds the live journal, the capture store and the default runtime cache.
	VarLibDir string
	// Private (terminal and debug) runs use temporary history and capture stores
	// and leave the live ones untouched. The browser runtime store is shared.
	Private    bool
	GeoIPPaths geoip.Paths
	// Warnings receives non-fatal startup diagnostics; required (the command passes stderr).
	Warnings io.Writer
}

// Process is one composed plugin process: its stores, browser runtime, runner,
// components and process services.
type Process struct {
	Components
	Services []composition.ProcessService
	executor *runner.Engine
	runtime  *runtimehost.Handle
	captures *artifacts.Store
	history  *journal.Store
}

// Open opens the journal first and applies the configured history budget
// before any producer can append, then the capture store, the browser runtime
// and the runner, and composes the components. Browser runtime problems never
// fail Open: synthetic jobs report them while RUM keeps working.
func Open(opts Options) (*Process, error) {
	cfg := opts.Config
	path, err := historyPath(opts.VarLibDir, opts.Private)
	if err != nil {
		return nil, fmt.Errorf("initializing DEM history: %w", err)
	}
	history, err := journal.Open(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("opening DEM history: %w", err)
	}
	if err := history.EnforceHistoryRetention(context.Background(), cfg.History.Days, cfg.History.MaxBytes); err != nil {
		_ = history.Close()
		return nil, fmt.Errorf("initializing DEM history policy: %w", err)
	}
	artifactPath := ""
	if !opts.Private {
		artifactPath = filepath.Join(opts.VarLibDir, "dem", "artifacts")
	}
	captures, err := artifacts.Open(artifactPath)
	if err != nil {
		_ = history.Close()
		return nil, fmt.Errorf("opening DEM artifacts: %w", err)
	}
	if _, err := captures.Enforce(context.Background(), cfg.Artifacts.Days, cfg.Artifacts.MaxBytes); err != nil {
		// Inventory/cleanup failure does not invalidate the owned store or stop runs.
		// Process maintenance retries it, and the catalogue exposes its status.
		_, _ = fmt.Fprintf(opts.Warnings, "initial DEM artifact cleanup failed: %v\n", err)
	}
	browserRuntime := runtimehost.Open(cfg.Runtime, opts.VarLibDir)
	syntheticHistory := synthetichistory.NewStore(history)
	executor, err := runner.New(browserRuntime.Source, syntheticHistory, captures, cfg.Browser.Concurrency)
	if err != nil {
		_ = browserRuntime.Close()
		_ = captures.Close()
		_ = history.Close()
		return nil, fmt.Errorf("initializing DEM browser execution: %w", err)
	}
	var browserActivation func(map[string]any) ([]collectorapi.ActivationDependency, error)
	var updater *runtimehost.Updater
	if browserRuntime.Host != nil {
		browserActivation = browserRuntime.Host.Dependencies
		updater = browserRuntime.Host.NewUpdater(executor, cfg.Runtime.ReleaseLoader("", true))
	}
	components := compose(Dependencies{
		GeoIPPaths:        opts.GeoIPPaths,
		History:           history,
		Artifacts:         captures,
		Executor:          executor,
		BrowserActivation: browserActivation,
		ConfigProvider:    opts.LoadConfig,
	}, cfg, syntheticHistory)
	services := []composition.ProcessService{components.Retention}
	if browserRuntime.Host != nil {
		services = append(services, browserRuntime.Host)
	}
	if updater != nil {
		services = append(services, updater)
	}
	return &Process{
		Components: components,
		Services:   services,
		executor:   executor,
		runtime:    browserRuntime,
		captures:   captures,
		history:    history,
	}, nil
}

// Fatal reports an unverified browser process tree. The command must then exit
// without closing the stores, so nothing deletes files a surviving child might use.
func (p *Process) Fatal() <-chan error { return p.executor.Fatal() }

// Close releases the runtime, the capture store and the journal, in that order.
// Only a joined, successful Agent run may call it: an error or forced recovery
// can leave active consumers, and process exit then owns their resources. It
// stops at the first failure, which the command reports before exiting.
func (p *Process) Close() error {
	if err := p.runtime.Close(); err != nil {
		return fmt.Errorf("closing DEM runtime: %w", err)
	}
	if err := p.captures.Close(); err != nil {
		return fmt.Errorf("closing DEM artifacts: %w", err)
	}
	if err := p.history.Close(); err != nil {
		return fmt.Errorf("closing DEM history: %w", err)
	}
	return nil
}

// historyPath selects the private temporary journal for terminal and debug runs.
func historyPath(varLib string, private bool) (string, error) {
	if private {
		return "", nil
	}
	if strings.TrimSpace(varLib) == "" {
		return "", fmt.Errorf("NETDATA_LIB_DIR is required outside terminal/debug mode")
	}
	return filepath.Join(varLib, "dem", "journal"), nil
}
