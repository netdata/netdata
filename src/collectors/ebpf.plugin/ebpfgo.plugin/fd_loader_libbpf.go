//go:build netdata_ebpf_libbpf

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// tryLoadFDPlan opens, prepares, loads, attaches, and configures a single plan.
// On any failure the partially-initialised runtime is closed before returning.
// Matches tryLoadCachestatPlan / tryLoadDCStatPlan / tryLoadSocketPlan.
func tryLoadFDPlan(cfg FDLegacyConfig, plan LoadPlan) (*FDLegacyHandle, error) {
	coreSupported := libbpfloader.FDSupportsCore()
	rt, err := libbpfloader.NewFDRuntime(plan.ObjectPath, plan.LoadMode == LoadCore && coreSupported)
	if err != nil {
		return nil, err
	}

	// The close target is needed here, not just at attach time: the base object
	// ships one close program per kernel symbol name, so prepare() has to
	// autoload the one matching this host and disable the other.
	if err := rt.Prepare(cfg.PidTableSize, cfg.MapsPerCore, cfg.Targets.Close); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Attach(cfg.Targets.Open, cfg.Targets.Close); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.UpdateController(cfg.AppsEnabled || cfg.CgroupsEnabled, cfg.AppsLevel); err != nil {
		rt.Close()
		return nil, err
	}

	// The shared memory publisher is opened lazily on the first publish call
	// (see runFDGlobalCollector) so hosts with neither apps nor cgroups
	// integration enabled never reserve the segment.

	return &FDLegacyHandle{
		Plan:           plan,
		Runtime:        rt,
		SharedMemory:   nil,
		UpdateEvery:    cfg.UpdateEvery,
		ConfigFound:    cfg.ConfigFound,
		PidTableSize:   cfg.PidTableSize,
		MapsPerCore:    cfg.MapsPerCore,
		AppsEnabled:    cfg.AppsEnabled,
		CgroupsEnabled: cfg.CgroupsEnabled,
		AppsLevel:      cfg.AppsLevel,
		ReportErrors:   cfg.ReportErrors,
	}, nil
}

func LoadFDLegacy(cfg FDLegacyConfig) (*FDLegacyHandle, error) {
	// resolveFDLegacyConfig() only warns when the symbols cannot be resolved, so
	// that one module's missing kernel symbol never aborts plugin startup for the
	// others.  Loading without them is what actually cannot proceed.
	if cfg.Targets.Open == "" || cfg.Targets.Close == "" {
		return nil, fmt.Errorf("fd: attach targets are unresolved; refusing to load")
	}

	versionStr, err := KernelVersionString()
	if err != nil {
		return nil, fmt.Errorf("kernel version string: %w", err)
	}
	rejected, err := KernelRejected(versionStr, filepath.Join(cfg.PluginsDir, "ebpf_kernel_reject_list.txt"))
	if err != nil {
		return nil, fmt.Errorf("kernel reject check: %w", err)
	}
	if err := CanLoadCode(cfg.KernelVersion, cfg.IsRHF, rejected, IsRoot(), "ebpf-go.plugin"); err != nil {
		return nil, err
	}

	plan := BuildFDLegacyPlan(cfg)
	if !libbpfloader.FDSupportsCore() {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		if int(selector) > fdMaxBaseSelector {
			selector = uint32(fdMaxBaseSelector)
		}
		plan.ObjectPath = BuildObjectPathWithFlavor(
			cfg.PluginsDir, selector, "fd", plan.IsReturn, cfg.IsRHF, ObjectFlavorBase)
		plan.LoadMode = LoadLegacy
		// No fallback in legacy mode: there is only one object.
		return tryLoadFDPlan(cfg, plan)
	}

	plans := buildFallbackPlans(plan, cfg.PluginsDir, cfg.IsRHF, "fd", fdMaxBaseSelector)
	var lastErr error
	for i, fp := range plans {
		handle, err := tryLoadFDPlan(cfg, fp)
		if err == nil {
			return handle, nil
		}
		lastErr = err
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr,
				"ebpf-go.plugin: fd %s unavailable (%v), trying fallback\n",
				filepath.Base(fp.ObjectPath), err)
		}
	}
	return nil, lastErr
}

func LoadFDLegacyFromSystem() (*FDLegacyHandle, error) {
	cfg, err := resolveFDLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadFDLegacy(cfg)
}
