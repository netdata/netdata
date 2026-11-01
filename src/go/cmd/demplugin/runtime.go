// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// runRuntimeCommand is dispatched before cli.Parse and never enters the plugin
// protocol. Executable candidates require publisher-authenticated release metadata.
func runRuntimeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (handled bool, retErr error) {
	if handled, err := runNativeRuntimeCommand(ctx, args, stdout, stderr); handled {
		return true, err
	}
	if len(args) == 0 || args[0] != "runtime" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: dem.plugin runtime {install|update|repair|uninstall|preload|status|qualify} [flags]")
	}
	command := args[1]
	switch command {
	case "install", "update", "repair", "uninstall", "preload", "status", "qualify":
	default:
		return true, fmt.Errorf("unknown runtime command %q", command)
	}
	preparing := command == "install" || command == "update" || command == "repair" || command == "preload"
	flags := flag.NewFlagSet("runtime "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	releasePath := flags.String("release", "", "local signed release descriptor; otherwise use configured latest URL")
	manualUpdates := flags.Bool("manual-updates", false, "disable automatic daily update preparation (install/repair); activation always requires restart")
	cache := flags.String("cache", "", "managed installation directory")
	root := flags.String("root", "", "preload final root (must be new or empty; never move it)")
	archive := flags.String("archive", "", "offline archive checked against the trusted release descriptor")
	allowDownload := flags.Bool("allow-download", false, "explicitly download canonical package-signing keys, signed release metadata and runtime bytes; install/repair enables automatic preparation unless --manual-updates")
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return true, err
	}
	if flags.NArg() != 0 {
		return true, errors.New("unexpected positional runtime arguments")
	}
	if *configDir != "" && !filepath.IsAbs(*configDir) {
		return true, errors.New("--config-dir must be absolute")
	}
	if preparing {
		if *releasePath != "" && !filepath.IsAbs(*releasePath) {
			return true, errors.New("--release must be absolute")
		}
		if (*archive != "") == *allowDownload {
			return true, errors.New("provide either --archive or --allow-download for install/update/repair/preload")
		}
		if *archive != "" && *releasePath == "" {
			return true, errors.New("offline archive requires an absolute --release path")
		}
	} else if *releasePath != "" || *archive != "" || *allowDownload {
		return true, errors.New("--release, --archive and --allow-download are only valid for install/update/repair/preload")
	}
	if *manualUpdates && command != "install" && command != "repair" {
		return true, errors.New("--manual-updates is only valid for install/repair; update preserves the installed policy")
	}
	if *archive != "" && !filepath.IsAbs(*archive) {
		return true, errors.New("--archive must be absolute")
	}
	if command == "preload" {
		if !filepath.IsAbs(*root) {
			return true, errors.New("preload requires an absolute --root final directory")
		}
		if *cache != "" {
			return true, errors.New("preload uses --root, not --cache")
		}
	} else if *root != "" {
		return true, errors.New("--root is only valid for preload")
	}
	var confDirs []string
	if *configDir != "" {
		confDirs = []string{*configDir}
	}
	config, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs(confDirs))
	if err != nil {
		return true, err
	}
	cfg := config.Runtime
	if *cache != "" {
		cfg.CachePath = *cache
	}
	report := runtimeCommandReport{Command: command, Mode: cfg.Mode, State: "unavailable", DownloadAllowed: *allowDownload, ReleasePath: *releasePath}
	if command == "repair" {
		report.State = "unknown"
	}
	defer func() {
		if retErr != nil {
			report.SetupError = retErr.Error()
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		retErr = errors.Join(retErr, encoder.Encode(report))
	}()
	if (cfg.Mode == "preloaded" || cfg.Mode == "native") && command != "preload" {
		if command != "status" && command != "qualify" {
			if cfg.Mode == "native" {
				return true, errors.New("native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package")
			}
			return true, errors.New("install/update/repair/uninstall require runtime.mode: managed")
		}
		handle := dem.OpenRuntime(cfg, "")
		defer func() {
			if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
				retErr = errors.Join(retErr, handle.Close())
			}
		}()
		if handle.SetupError != nil {
			return true, handle.SetupError
		}
		if cfg.Mode == "native" {
			status := handle.Host.Status()
			report.Runtime = status.Running
			report.Native = status.Native
			report.Notice = "Native package files are owned by the package manager. Stop standalone consumers before package replacement or removal."
		} else {
			report.Runtime = releaseIdentity(*handle.Manifest)
			report.PreloadPath = cfg.PreloadPath
		}
		report.Generation = report.Runtime.SHA256
		report.State = "installed"
		if command == "qualify" {
			report.Qualification, retErr = qualifyRuntime(ctx, handle.Source)
			if retErr == nil {
				report.State = "qualified"
			}
		}
		return true, retErr
	}
	varlib := os.Getenv("NETDATA_LIB_DIR")
	if varlib == "" {
		varlib = buildinfo.VarLibDir
	}
	if varlib == "" {
		varlib = buildinfo.DefaultVarLibDir
	}
	if command == "preload" {
		entries, err := os.ReadDir(*root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
		if len(entries) != 0 {
			return true, errors.New("preload root must be empty; use a new final root and stop all consumers before replacement")
		}
		cfg.CachePath = *root
	}
	storeConfig, err := cfg.StoreConfig(varlib)
	if err != nil {
		return true, err
	}
	report.CachePath = storeConfig.Path
	if !preparing {
		if _, err := os.Stat(storeConfig.Path); err != nil {
			if command == "status" && !errors.Is(err, os.ErrNotExist) {
				report.State = "unknown"
			}
			return true, fmt.Errorf("runtime installation: %w", err)
		}
	}
	store, err := runtime.Open(storeConfig)
	if err != nil {
		if command == "status" {
			report.State = "unknown"
		}
		return true, err
	}
	sealed := false
	defer func() {
		if !sealed && command != "status" {
			status, err := store.Inspect()
			report.Storage = &status
			report.StorageError = errorString(err)
			if preparing {
				report.selectedInstallation(status)
				if retErr != nil && command == "repair" {
					report.Notice = "Repair failed; selected installation state is shown separately from candidate progress. A cleared selection is not restored. Stop and drain all consumers before retrying repair."
				}
			}
		}
		if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
			retErr = errors.Join(retErr, store.Close())
		}
	}()
	if preparing {
		options := runtime.ReleaseInstallOptions{Operation: runtime.ReleaseOperation(command)}
		if command == "preload" {
			options.Operation = runtime.InstallRelease
		}
		if options.Operation == runtime.InstallRelease || options.Operation == runtime.RepairRelease {
			options.Updates = runtime.AutomaticUpdates
			if *manualUpdates || command == "preload" {
				options.Updates = runtime.ManualUpdates
			}
			if options.Updates == runtime.AutomaticUpdates {
				fmt.Fprintln(stderr, "Runtime installation enables daily automatic update preparation. Activation requires a DEM restart; use --manual-updates to disable automatic preparation.")
			}
		}
		candidate, err := store.BeginPublishedInstall(ctx, options, cfg.ReleaseLoader(*releasePath, *allowDownload))
		if err != nil {
			return true, err
		}
		if candidate == nil {
			report.Notice = "Selected release is unchanged. Already running processes keep their startup release until restart."
			return true, nil
		}
		m := candidate.Manifest()
		report.Candidate = releaseIdentity(m)
		report.CandidateState = "requested"
		defer func() {
			if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
				retErr = errors.Join(retErr, candidate.Close())
			}
		}()
		if *archive != "" {
			err = candidate.Import(ctx, *archive)
		} else {
			err = candidate.Download(ctx)
		}
		if err != nil {
			return true, err
		}
		report.CandidateState = "prepared"
		report.Qualification, err = qualifyRuntime(ctx, runtimehost.Borrow(candidate))
		if err != nil {
			return true, err
		}
		if command == "preload" {
			if err = candidate.SealPreload(); err != nil {
				return true, err
			}
			sealed = true
			report.Runtime = releaseIdentity(m)
			report.Generation = m.Archive.SHA256
			report.CachePath = ""
			report.PreloadPath = storeConfig.Path
			report.PreloadedConfig = map[string]any{"runtime": map[string]any{"mode": "preloaded", "preload_path": storeConfig.Path}}
			report.Notice = "Preload is qualified for this CLI user/environment and bound to this final root. Keep it immutable and private. Stop all consumers before replacement or removal; this is not a native package transaction. Restart DEM to activate it."
		} else {
			if err = candidate.Select(); err != nil {
				return true, err
			}
			report.Notice = "Runtime installed and selected. Running DEM processes keep their acquired release; restart DEM to activate this selection."
		}
		report.CandidateState = "selected"
		report.State = "installed"
		return true, nil
	}
	switch command {
	case "uninstall":
		if err = store.Uninstall(ctx); err != nil {
			return true, err
		}
		report.State = "uninstalled"
	case "status":
		status, err := store.Inspect()
		report.Storage = &status
		report.StorageError = errorString(err)
		report.selectedInstallation(status)
		report.Notice = "Selected installation is shown; already running DEM processes may retain an older release until restart. Metadata damage requires explicit repair after stopping and draining consumers."
		if err != nil {
			return true, err
		}
		if status.Selected == nil {
			return true, runtime.ErrUnavailable
		}
	case "qualify":
		consumer, err := store.AcquireSelected()
		if err != nil {
			return true, err
		}
		defer func() {
			if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
				retErr = errors.Join(retErr, consumer.Close())
			}
		}()
		m := consumer.Manifest()
		report.Runtime = releaseIdentity(m)
		report.Generation = m.Archive.SHA256
		report.State = "installed"
		report.Qualification, err = qualifyRuntime(ctx, runtimehost.Borrow(consumer))
		if err != nil {
			return true, err
		}
		report.State = "qualified"
	}
	return true, nil
}

type runtimeCommandReport struct {
	DownloadAllowed bool                       `json:"download_allowed"`
	Command         string                     `json:"command"`
	Mode            string                     `json:"mode"`
	State           string                     `json:"state"`
	ReleasePath     string                     `json:"release_path,omitempty"`
	CachePath       string                     `json:"cache_path,omitempty"`
	Generation      string                     `json:"generation,omitempty"`
	Runtime         *synthetic.RuntimeIdentity `json:"runtime,omitempty"`
	Candidate       *synthetic.RuntimeIdentity `json:"candidate,omitempty"`
	CandidateState  string                     `json:"candidate_state,omitempty"`
	SetupError      string                     `json:"setup_error,omitempty"`
	Storage         *runtime.Status            `json:"storage,omitempty"`
	StorageError    string                     `json:"storage_error,omitempty"`
	Native          *runtime.NativeStatus      `json:"native,omitempty"`
	Qualification   *runner.Qualification      `json:"qualification,omitempty"`
	PreloadedConfig map[string]any             `json:"preloaded_config,omitempty"`
	PreloadPath     string                     `json:"preload_path,omitempty"`
	Notice          string                     `json:"notice,omitempty"`
}

// selectedInstallation reports executable selection independently of a candidate
// and of damage to unrelated retained generations.
func (r *runtimeCommandReport) selectedInstallation(status runtime.Status) {
	r.Runtime = nil
	r.Generation = ""
	r.State = "unavailable"
	if status.Selected != nil {
		r.Runtime = releaseIdentity(*status.Selected)
		r.Generation = status.Selected.Archive.SHA256
		r.State = "installed"
	}
	if status.SelectionError != "" {
		r.State = "damaged"
	}
	if status.InventoryError != "" {
		r.State = "unknown"
	}
}

func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
func releaseIdentity(m runtime.Manifest) *synthetic.RuntimeIdentity {
	return &synthetic.RuntimeIdentity{Version: m.Version, SHA256: m.Archive.SHA256, Contract: m.Contract}
}
