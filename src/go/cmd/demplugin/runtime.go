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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// runRuntimeCommand is dispatched before cli.Parse and never enters the plugin
// protocol. Executable candidates require administrator-trusted local metadata.
func runRuntimeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (handled bool, retErr error) {
	if len(args) == 0 || args[0] != "runtime" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: dem.plugin runtime {install|update|uninstall|preload|status|qualify} [flags]")
	}
	command := args[1]
	switch command {
	case "install", "update", "uninstall", "preload", "status", "qualify":
	default:
		return true, fmt.Errorf("unknown runtime command %q", command)
	}
	preparing := command == "install" || command == "update" || command == "preload"
	flags := flag.NewFlagSet("runtime "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	manifest := flags.String("manifest", "", "administrator-trusted release descriptor (required for install/update/preload)")
	cache := flags.String("cache", "", "managed installation directory")
	root := flags.String("root", "", "preload final root (must be new or empty; never move it)")
	archive := flags.String("archive", "", "offline archive checked against the trusted release descriptor")
	allowDownload := flags.Bool("allow-download", false, "explicitly download the candidate from the trusted descriptor URL")
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
		if !filepath.IsAbs(*manifest) {
			return true, errors.New("install/update/preload requires an absolute --manifest path")
		}
		if (*archive != "") == *allowDownload {
			return true, errors.New("provide either --archive or --allow-download for install/update/preload")
		}
	} else if *manifest != "" || *archive != "" || *allowDownload {
		return true, errors.New("--manifest, --archive and --allow-download are only valid for install/update/preload")
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
	report := runtimeCommandReport{Command: command, Mode: cfg.Mode, State: "unavailable", DownloadAllowed: *allowDownload, ManifestPath: *manifest}
	defer func() {
		if retErr != nil {
			report.SetupError = retErr.Error()
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		retErr = errors.Join(retErr, encoder.Encode(report))
	}()
	if cfg.Mode == "preloaded" && command != "preload" {
		if command != "status" && command != "qualify" {
			return true, errors.New("install/update/uninstall require runtime.mode: managed")
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
		report.Runtime = releaseIdentity(*handle.Manifest)
		report.Generation = handle.Manifest.Archive.SHA256
		report.PreloadPath = cfg.PreloadPath
		report.State = "installed"
		if command == "qualify" {
			report.Qualification, retErr = qualifyRuntime(ctx, handle.Source)
			if retErr == nil {
				report.State = "qualified"
			}
		}
		return true, retErr
	}
	var trusted *runtime.Manifest
	if preparing {
		trusted, err = dem.LoadRuntimeManifest(*manifest)
		if err != nil {
			return true, err
		}
		report.Runtime = releaseIdentity(*trusted)
		report.Generation = trusted.Archive.SHA256
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
			return true, fmt.Errorf("runtime installation: %w", err)
		}
	}
	store, err := runtime.Open(storeConfig)
	if err != nil {
		return true, err
	}
	sealed := false
	defer func() {
		if !sealed {
			status, err := store.Inspect()
			report.Storage = &status
			report.StorageError = errorString(err)
		}
		if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
			retErr = errors.Join(retErr, store.Close())
		}
	}()
	if preparing {
		candidate, err := store.BeginInstall(ctx, *trusted)
		if err != nil {
			return true, err
		}
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
		report.State = "prepared"
		report.Qualification, err = qualifyRuntime(ctx, adminRuntimeSource{candidate})
		if err != nil {
			return true, err
		}
		if command == "preload" {
			if err = candidate.SealPreload(); err != nil {
				return true, err
			}
			sealed = true
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
		if err != nil {
			return true, err
		}
		if status.Selected == nil {
			return true, runtime.ErrUnavailable
		}
		report.Runtime = releaseIdentity(*status.Selected)
		report.Generation = status.Selected.Archive.SHA256
		report.State = "installed"
		report.Notice = "Selected installation is shown; already running DEM processes may retain an older release until restart."
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
		report.Qualification, err = qualifyRuntime(ctx, adminRuntimeSource{consumer})
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
	ManifestPath    string                     `json:"manifest_path,omitempty"`
	CachePath       string                     `json:"cache_path,omitempty"`
	Generation      string                     `json:"generation,omitempty"`
	Runtime         *synthetic.RuntimeIdentity `json:"runtime,omitempty"`
	SetupError      string                     `json:"setup_error,omitempty"`
	Storage         *runtime.Status            `json:"storage,omitempty"`
	StorageError    string                     `json:"storage_error,omitempty"`
	Qualification   *runtimeQualification      `json:"qualification,omitempty"`
	PreloadedConfig map[string]any             `json:"preloaded_config,omitempty"`
	PreloadPath     string                     `json:"preload_path,omitempty"`
	Notice          string                     `json:"notice,omitempty"`
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

type runtimeConsumer interface {
	Manifest() runtime.Manifest
	BeginUse() (*runtime.Use, error)
}
type adminRuntimeSource struct{ consumer runtimeConsumer }

func (s adminRuntimeSource) BeginUse() (runner.Use, error) {
	use, err := s.consumer.BeginUse()
	if err != nil {
		return nil, err
	}
	return adminRuntimeUse{use: use, identity: *releaseIdentity(s.consumer.Manifest())}, nil
}

type adminRuntimeUse struct {
	use      *runtime.Use
	identity synthetic.RuntimeIdentity
}

func (u adminRuntimeUse) Paths() runner.Paths {
	return runner.Paths{Node: u.use.Paths.Node, EntryPoint: u.use.Paths.EntryPoint, Identity: u.identity}
}
func (u adminRuntimeUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u adminRuntimeUse) Finish(drained bool) error          { return u.use.Finish(drained) }
