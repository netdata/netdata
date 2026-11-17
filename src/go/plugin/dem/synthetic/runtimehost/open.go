// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
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
	// SetupError reports why the runtime is unusable. RUM keeps working.
	SetupError error
	identity   synthetic.RuntimeIdentity
	store      *browserruntime.Store
	native     *browserruntime.Native
	closeOnce  sync.Once
	closeErr   error
}

// Open never fails: setup problems leave synthetic jobs waiting or failing
// with SetupError while the rest of the plugin runs.
func Open(cfg Config, varLibPath string) *Handle {
	h := &Handle{
		SetupError: cfg.Validate(),
	}
	if cfg.Mode == ModePreloaded && !filepath.IsAbs(cfg.PreloadPath) {
		h.SetupError = errors.Join(h.SetupError, errors.New("runtime.preload_path must be absolute"))
	}
	if !browserruntime.SupportedHost() {
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
		paths, manifest, err := browserruntime.ReadPreload(path)
		if err == nil {
			h.identity = manifest.Identity()
			h.Source = preloadedSource{runnerPaths(paths, h.identity)}
			return
		}
		h.SetupError = err
	}
	h.Source = unavailableSource{h.SetupError}
}

func (h *Handle) openNative() {
	if h.SetupError == nil {
		h.native, h.SetupError = browserruntime.OpenNative(browserruntime.DefaultNativeConfig())
	}
	if h.SetupError != nil {
		h.Host = newUnavailable(ModeNative, h.SetupError)
	} else {
		h.Host = newNative(h.native)
	}
	h.adoptHost()
}

func (h *Handle) openManaged(cfg Config, varLibPath string) {
	if h.SetupError == nil {
		var storeConfig browserruntime.Config
		storeConfig, h.SetupError = cfg.StoreConfig(varLibPath)
		if h.SetupError == nil {
			h.store, h.SetupError = browserruntime.Open(storeConfig)
		}
	}
	if h.SetupError != nil {
		h.Host = newUnavailable(ModeManaged, h.SetupError)
	} else {
		h.Host = newManaged(h.store)
	}
	h.adoptHost()
}

// adoptHost makes the host the process source and records its release.
func (h *Handle) adoptHost() {
	h.SetupError = h.Host.Ready()
	h.Source = h.Host
	if h.Host.identity != nil {
		h.identity = *h.Host.identity
	}
}

// Identity is the release the process runs; zero when SetupError is set.
func (h *Handle) Identity() synthetic.RuntimeIdentity { return h.identity }

// NativeStatus inspects the native package's ownership proof; nil unless native
// mode opened its proof root. An inspection failure is reported in the status.
func (h *Handle) NativeStatus() *browserruntime.NativeStatus {
	if h.native == nil {
		return nil
	}
	status, _ := h.native.Inspect()
	return &status
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
