// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
)

// Handle owns the browser runtime resources of one plugin process. Close follows
// joined jobs and workers; it must not run while a supervised browser invocation
// may still use the cache.
type Handle struct {
	// Source serves every runner probe and invocation; when the runtime is
	// unusable, BeginUse reports why.
	Source runner.Source
	// Host holds the managed or native selection; preloads have none.
	Host *Host
	// Manifest is the verified preload descriptor; nil in other modes or when
	// the preload is unusable.
	Manifest *runtime.Manifest
	// SetupError reports why the runtime is unusable. RUM keeps working.
	SetupError error
	store      *runtime.Store
	native     *runtime.Native
	closeOnce  sync.Once
	closeErr   error
}

// Open never fails: setup problems leave synthetic jobs waiting or failing
// with SetupError while the rest of the plugin runs.
func Open(cfg Config, varLibPath string) *Handle {
	h := &Handle{SetupError: cfg.Validate()}
	if cfg.Mode == ModePreloaded && !filepath.IsAbs(cfg.PreloadPath) {
		h.SetupError = errors.Join(h.SetupError, errors.New("runtime.preload_path must be absolute"))
	}
	if !runtime.SupportedHost() {
		h.SetupError = errors.Join(h.SetupError, errors.New("browser runtime requires Linux amd64 or arm64"))
	}
	switch cfg.Mode {
	case ModePreloaded:
		h.openPreload(cfg.PreloadPath)
	case ModeNative:
		h.openNative()
	default:
		h.openManaged(cfg, varLibPath)
	}
	return h
}

func (h *Handle) openPreload(path string) {
	if h.SetupError == nil {
		paths, manifest, err := runtime.ReadPreload(path)
		if err == nil {
			h.Manifest = &manifest
			h.Source = preloadedSource{runnerPaths(paths, manifest.Identity())}
			return
		}
		h.SetupError = err
	}
	h.Source = unavailableSource{h.SetupError}
}

func (h *Handle) openNative() {
	if h.SetupError == nil {
		h.native, h.SetupError = runtime.OpenNative(runtime.DefaultNativeConfig())
	}
	if h.SetupError != nil {
		h.Host = NewUnavailable(ModeNative, h.SetupError)
	} else {
		h.Host = NewNative(h.native)
	}
	h.SetupError = h.Host.Ready()
	h.Source = h.Host
}

func (h *Handle) openManaged(cfg Config, varLibPath string) {
	if h.SetupError == nil {
		var storeConfig runtime.Config
		storeConfig, h.SetupError = cfg.StoreConfig(varLibPath)
		if h.SetupError == nil {
			h.store, h.SetupError = runtime.Open(storeConfig)
		}
	}
	if h.SetupError != nil {
		h.Host = NewUnavailable(ModeManaged, h.SetupError)
	} else {
		h.Host = NewManaged(h.store)
	}
	h.SetupError = h.Host.Ready()
	h.Source = h.Host
}

// Close joins the host (and its updater) before closing the storage it opened.
func (h *Handle) Close() error {
	h.closeOnce.Do(func() {
		if h.Host != nil {
			h.closeErr = h.Host.Close()
		}
		if h.store != nil {
			h.closeErr = errors.Join(h.closeErr, h.store.Close())
		}
		if h.native != nil {
			h.closeErr = errors.Join(h.closeErr, h.native.Close())
		}
	})
	return h.closeErr
}
