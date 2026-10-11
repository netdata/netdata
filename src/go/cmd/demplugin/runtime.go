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
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// runRuntimeCommand is dispatched before cli.Parse. It never enters the plugin
// protocol and only accepts administrator inputs from the command line/config.
func runRuntimeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (handled bool, retErr error) {
	if len(args) == 0 || args[0] != "runtime" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: dem.plugin runtime {prepare|import|preload|status|qualify} [flags]")
	}
	command := args[1]
	switch command {
	case "prepare", "import", "preload", "status", "qualify":
	default:
		return true, fmt.Errorf("unknown runtime command %q", command)
	}
	flags := flag.NewFlagSet("runtime "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	assets := flags.String("assets", filepath.Join(executable.Directory, "dem"), "installed DEM adapter directory")
	manifest := flags.String("manifest", "", "administrator-trusted runtime manifest (default: installed architecture manifest)")
	cache := flags.String("cache", "", "managed cache directory")
	root := flags.String("root", "", "preload final root (must be new or empty; never move it)")
	archive := flags.String("archive", "", "offline archive checked against the trusted manifest")
	allowDownload := flags.Bool("allow-download", true, "allow prepare/preload to use the installed manifest URL")
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return true, err
	}
	if flags.NArg() != 0 {
		return true, errors.New("unexpected positional runtime arguments")
	}
	visited := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if !filepath.IsAbs(*assets) {
		return true, errors.New("--assets must be absolute")
	}
	if *configDir != "" && !filepath.IsAbs(*configDir) {
		return true, errors.New("--config-dir must be absolute")
	}
	if (command == "import") != (*archive != "") && command != "preload" {
		return true, errors.New("--archive is required for import and only valid for import/preload")
	}
	if command == "preload" && *root == "" {
		return true, errors.New("preload requires an explicit --root final directory")
	}
	if command != "preload" && *root != "" {
		return true, errors.New("--root is only valid for preload")
	}
	if command == "preload" && *cache != "" {
		return true, errors.New("preload uses --root, not --cache")
	}
	if *archive != "" && !filepath.IsAbs(*archive) {
		return true, errors.New("--archive must be absolute")
	}
	paths := multipath.New(os.Getenv("NETDATA_USER_CONFIG_DIR"), buildinfo.UserConfigDir, os.Getenv("NETDATA_STOCK_CONFIG_DIR"), buildinfo.StockConfigDir)
	if *configDir != "" {
		paths = multipath.New(*configDir)
	}
	config, err := dem.LoadConfig(paths)
	if err != nil {
		return true, err
	}
	cfg := config.Runtime
	if visited["manifest"] {
		cfg.ManifestPath = *manifest
	}
	if visited["cache"] {
		cfg.CachePath = *cache
	}
	if visited["allow-download"] {
		cfg.AllowDownload = *allowDownload
	}
	report := runtimeCommandReport{Command: command, Mode: cfg.Mode, State: "unavailable", DownloadAllowed: cfg.AllowDownload}
	defer func() {
		if retErr != nil {
			report.SetupError = retErr.Error()
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		retErr = errors.Join(retErr, encoder.Encode(report))
	}()
	if cfg.Mode == "external" && command != "preload" {
		if command != "status" && command != "qualify" {
			return true, errors.New("prepare/import require runtime.mode: managed")
		}
		handle := dem.OpenRuntime(cfg, *assets, "")
		defer handle.Close()
		if handle.SetupError != nil {
			return true, handle.SetupError
		}
		report.State = "external_unverified"
		if command == "qualify" {
			report.Qualification, retErr = qualifyRuntime(ctx, *assets, handle.Source)
			if retErr == nil {
				report.State = "qualified"
			}
		}
		return true, retErr
	}
	report.ManifestPath = dem.RuntimeManifestPath(cfg, *assets)
	trusted, err := dem.LoadRuntimeManifest(report.ManifestPath)
	if err != nil {
		return true, err
	}
	report.Generation = trusted.Archive.SHA256
	varlib := os.Getenv("NETDATA_LIB_DIR")
	if varlib == "" {
		varlib = buildinfo.VarLibDir
	}
	if varlib == "" {
		varlib = buildinfo.DefaultVarLibDir
	}
	if command == "preload" {
		if !filepath.IsAbs(*root) {
			return true, errors.New("--root must be absolute")
		}
		// A fresh final root prevents accidental replacement beneath external users.
		entries, err := os.ReadDir(*root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
		if len(entries) != 0 {
			return true, errors.New("preload root must be empty; use a new final root and stop consumers before replacing an old preload")
		}
		cfg.CachePath = *root
	}
	storeConfig, err := cfg.StoreConfig(varlib)
	if err != nil {
		return true, err
	}
	report.CachePath = storeConfig.Path
	if command == "status" || command == "qualify" {
		if _, err := os.Stat(storeConfig.Path); err != nil {
			return true, fmt.Errorf("runtime cache: %w", err)
		}
	}
	store, err := runtime.Open(storeConfig)
	if err != nil {
		return true, err
	}
	defer func() {
		status, err := store.Inspect()
		report.Storage = &status
		report.StorageError = errorString(err)
		if !errors.Is(retErr, ndexec.ErrTreeNotDrained) {
			retErr = errors.Join(retErr, store.Close())
		}
	}()
	switch command {
	case "prepare", "preload":
		if *archive != "" {
			err = store.Import(ctx, *trusted, *archive)
		} else if err = store.Available(*trusted); errors.Is(err, runtime.ErrUnavailable) {
			if !cfg.AllowDownload {
				err = errors.New("runtime downloads are disabled; use import with a verified offline archive")
			} else {
				err = store.Download(ctx, *trusted)
			}
		}
	case "import":
		err = store.Import(ctx, *trusted, *archive)
	}
	if err != nil {
		return true, err
	}
	if err = store.Available(*trusted); err != nil {
		return true, err
	}
	report.State = "prepared"
	switch command {
	case "qualify":
		report.Qualification, err = qualifyRuntime(ctx, *assets, adminRuntimeSource{store: store, manifest: *trusted})
		if err != nil {
			return true, err
		}
		report.State = "qualified"
	case "preload":
		use, err := store.BeginUse(*trusted)
		if err != nil {
			return true, err
		}
		report.ExternalConfig = map[string]any{"runtime": map[string]any{"mode": "external", "node_path": use.Paths.Node, "dependencies_path": use.Paths.Dependencies, "browser_path": use.Paths.Browser}}
		if err = use.Finish(true); err != nil {
			return true, err
		}
		report.Notice = "Preload paths are bound to this final root. Keep it outside managed-cache configuration; external files are not automatically retired. Run under the consuming service account and keep permissions private. Stop all consumers before replacing or removing this root. Prepared does not mean browser-qualified; run qualify as the service account."
	}
	return true, nil
}

type runtimeCommandReport struct {
	DownloadAllowed bool                  `json:"download_allowed"`
	Command         string                `json:"command"`
	Mode            string                `json:"mode"`
	State           string                `json:"state"`
	ManifestPath    string                `json:"manifest_path,omitempty"`
	CachePath       string                `json:"cache_path,omitempty"`
	Generation      string                `json:"generation,omitempty"`
	SetupError      string                `json:"setup_error,omitempty"`
	Storage         *runtime.Status       `json:"storage,omitempty"`
	StorageError    string                `json:"storage_error,omitempty"`
	Qualification   *runtimeQualification `json:"qualification,omitempty"`
	ExternalConfig  map[string]any        `json:"external_config,omitempty"`
	Notice          string                `json:"notice,omitempty"`
}

func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

type adminRuntimeSource struct {
	store    *runtime.Store
	manifest runtime.Manifest
}

func (s adminRuntimeSource) BeginUse() (runner.Use, error) {
	use, err := s.store.BeginUse(s.manifest)
	if err != nil {
		return nil, err
	}
	return adminRuntimeUse{use}, nil
}

type adminRuntimeUse struct{ use *runtime.Use }

func (u adminRuntimeUse) Paths() runner.Paths {
	return runner.Paths{Node: u.use.Paths.Node, Dependencies: u.use.Paths.Dependencies, Browser: u.use.Paths.Browser}
}
func (u adminRuntimeUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u adminRuntimeUse) Finish(drained bool) error          { return u.use.Finish(drained) }
