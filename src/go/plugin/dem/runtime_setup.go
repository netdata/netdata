// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// LoadRuntimeManifest reads installed administrator-controlled trust. An archive,
// monitor request, or download response can never replace this manifest.
func LoadRuntimeManifest(path string) (*runtime.Manifest, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("runtime manifest path must be absolute")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("installed runtime manifest: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("installed runtime manifest must be a regular file no larger than 1 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20+1))
	decoder.DisallowUnknownFields()
	var manifest runtime.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("installed runtime manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("installed runtime manifest has trailing content")
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("installed runtime manifest: %w", err)
	}
	if manifest.Platform != "linux/"+goruntime.GOARCH {
		return nil, errors.New("installed runtime manifest does not match this architecture")
	}
	return &manifest, nil
}

func RuntimeManifestPath(cfg RuntimeConfig, assetsPath string) string {
	if cfg.ManifestPath != "" {
		return cfg.ManifestPath
	}
	return filepath.Join(assetsPath, "runtime-"+goruntime.GOARCH+".json")
}

// RuntimeHandle owns process resources. Close follows joined jobs and workers;
// it must not run while a supervised browser invocation may still use the cache.
type RuntimeHandle struct {
	Source     runner.Source
	Host       *runtimehost.Host
	Store      *runtime.Store
	Manifest   *runtime.Manifest
	SetupError error
	closeOnce  sync.Once
	closeErr   error
}

func OpenRuntime(cfg RuntimeConfig, assetsPath, varLibPath string) *RuntimeHandle {
	result := &RuntimeHandle{}
	if err := cfg.validate(); err != nil {
		result.SetupError = err
	}
	if cfg.Mode == "preloaded" && !filepath.IsAbs(cfg.PreloadPath) {
		result.SetupError = errors.Join(result.SetupError, errors.New("runtime.preload_path must be absolute"))
	}
	if goruntime.GOOS != "linux" || (goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64") {
		result.SetupError = errors.Join(result.SetupError, errors.New("browser runtime requires Linux amd64 or arm64"))
	}
	if result.SetupError == nil {
		result.Manifest, result.SetupError = LoadRuntimeManifest(RuntimeManifestPath(cfg, assetsPath))
	}
	if cfg.Mode == "preloaded" {
		if result.SetupError == nil {
			var paths runtime.Paths
			paths, result.SetupError = runtime.ReadPreload(cfg.PreloadPath, *result.Manifest)
			if result.SetupError == nil {
				result.Source = preloadedRuntime{paths: runner.Paths{Node: paths.Node, Dependencies: paths.Dependencies, Browser: paths.Browser}}
			}
		}
		if result.SetupError != nil {
			result.Source = unavailableRuntime{result.SetupError}
		}
		return result
	}
	if result.SetupError == nil {
		storeConfig, err := cfg.StoreConfig(varLibPath)
		if err == nil {
			result.Store, err = runtime.Open(storeConfig)
		}
		result.SetupError = err
	}
	result.Host = runtimehost.New(runtimehost.Config{Store: result.Store, Manifest: result.Manifest, Unavailable: result.SetupError, AllowDownload: cfg.AllowDownload})
	result.Source = result.Host
	return result
}

func (h *RuntimeHandle) Close() error {
	h.closeOnce.Do(func() {
		if h.Host != nil {
			h.closeErr = h.Host.Close()
		}
		if h.Store != nil {
			h.closeErr = errors.Join(h.closeErr, h.Store.Close())
		}
	})
	return h.closeErr
}

type unavailableRuntime struct{ err error }

func (s unavailableRuntime) BeginUse() (runner.Use, error) { return nil, s.err }

// A verified preload remains owned by the administrator, outside managed GC.
type preloadedRuntime struct{ paths runner.Paths }

func (s preloadedRuntime) BeginUse() (runner.Use, error) { return preloadedUse{s.paths}, nil }

type preloadedUse struct{ paths runner.Paths }

func (u preloadedUse) Paths() runner.Paths                { return u.paths }
func (u preloadedUse) Completion() *ndexec.TreeCompletion { return nil }
func (u preloadedUse) Finish(bool) error                  { return nil }
