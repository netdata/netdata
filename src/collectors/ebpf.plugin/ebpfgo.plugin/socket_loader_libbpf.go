//go:build netdata_ebpf_libbpf

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// tryLoadSocketPlan opens, prepares, loads, and attaches a single plan.
// On any failure the partially-initialised runtime is closed before returning.
func tryLoadSocketPlan(cfg SocketLegacyConfig, plan LoadPlan) (*SocketLegacyHandle, error) {
	rt, err := libbpfloader.NewSocketRuntime(plan.ObjectPath, plan.LoadMode == LoadCore)
	if err != nil {
		return nil, err
	}

	if err := rt.Prepare(cfg.MapsPerCore); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Attach(); err != nil {
		rt.Close()
		return nil, err
	}

	return &SocketLegacyHandle{
		Plan:        plan,
		Runtime:     rt,
		UpdateEvery: cfg.UpdateEvery,
		ConfigFound: cfg.ConfigFound,
		MapsPerCore: cfg.MapsPerCore,
	}, nil
}

// socketFallbackPlans returns plans in preference order: primary first, then
// progressively less demanding flavors.  This implements the fallback promised
// by ebpf.d.conf: "If the requested flavor is not available on this system,
// the collector falls back to tracing."
func socketFallbackPlans(primary LoadPlan, cfg SocketLegacyConfig) []LoadPlan {
	plans := []LoadPlan{primary}

	if primary.Flavor == ObjectFlavorArena {
		// arena → buffer (same load mode, same kernel version selector)
		fb := primary
		fb.Flavor = ObjectFlavorBuffer
		fb.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, primary.Selector, "socket", false, cfg.IsRHF, ObjectFlavorBuffer)
		plans = append(plans, fb)
	}

	if primary.Flavor != ObjectFlavorBase {
		// → base / tracing
		fb := primary
		fb.Flavor = ObjectFlavorBase
		fb.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, primary.Selector, "socket", false, cfg.IsRHF, ObjectFlavorBase)
		plans = append(plans, fb)
	}

	return plans
}

func LoadSocketLegacy(cfg SocketLegacyConfig) (*SocketLegacyHandle, error) {
	plan := BuildSocketLegacyPlan(cfg)
	coreSupported := libbpfloader.SupportsCore()
	if !coreSupported {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		plan = LoadPlan{
			KernelVersion: cfg.KernelVersion,
			IsRHF:         cfg.IsRHF,
			Selector:      selector,
			Flavor:        ObjectFlavorBase,
			ObjectPath:    BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "socket", false, cfg.IsRHF, ObjectFlavorBase),
			LoadMode:      LoadLegacy,
			ProgramMode:   LoadTrampoline,
		}
		// No fallback in legacy mode: there is only one object.
		return tryLoadSocketPlan(cfg, plan)
	}

	plans := socketFallbackPlans(plan, cfg)
	var lastErr error
	for i, fp := range plans {
		handle, err := tryLoadSocketPlan(cfg, fp)
		if err == nil {
			return handle, nil
		}
		lastErr = err
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr,
				"ebpf-go.plugin: socket %s unavailable (%v), trying fallback\n",
				filepath.Base(fp.ObjectPath), err)
		}
	}
	return nil, lastErr
}

func LoadSocketLegacyFromSystem() (*SocketLegacyHandle, error) {
	cfg, err := resolveSocketLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadSocketLegacy(cfg)
}
