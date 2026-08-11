//go:build netdata_ebpf_libbpf

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// tryLoadDCStatPlan opens, prepares, loads, attaches, and configures a single
// plan.  On any failure the partially-initialised runtime is closed before
// returning.  Matches tryLoadCachestatPlan / tryLoadSocketPlan / tryLoadDNSPlan.
func tryLoadDCStatPlan(cfg DCStatLegacyConfig, plan LoadPlan) (*DCStatLegacyHandle, error) {
	coreSupported := libbpfloader.DCStatSupportsCore()
	rt, err := libbpfloader.NewDCStatRuntime(plan.ObjectPath, plan.LoadMode == LoadCore && coreSupported)
	if err != nil {
		return nil, err
	}

	if err := rt.Prepare(cfg.PidTableSize, cfg.MapsPerCore); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Attach(cfg.Targets.LookupFast.Name, cfg.Targets.DLookup.Name); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.UpdateController(cfg.AppsEnabled || cfg.CgroupsEnabled, cfg.AppsLevel); err != nil {
		rt.Close()
		return nil, err
	}

	// The shared memory publisher is opened lazily on the first publish call
	// (see runDCStatGlobalCollector) so hosts with neither apps nor cgroups
	// integration enabled never reserve the segment.

	return &DCStatLegacyHandle{
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
	}, nil
}

func LoadDCStatLegacy(cfg DCStatLegacyConfig) (*DCStatLegacyHandle, error) {
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

	plan := BuildDCStatLegacyPlan(cfg)
	if !libbpfloader.DCStatSupportsCore() {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		if int(selector) > dcstatMaxBaseSelector {
			selector = uint32(dcstatMaxBaseSelector)
		}
		plan.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "dc", false, cfg.IsRHF, ObjectFlavorBase)
		plan.LoadMode = LoadLegacy
		// No fallback in legacy mode: there is only one object.
		return tryLoadDCStatPlan(cfg, plan)
	}

	plans := buildFallbackPlans(plan, cfg.PluginsDir, cfg.IsRHF, "dc", dcstatMaxBaseSelector)
	var lastErr error
	for i, fp := range plans {
		handle, err := tryLoadDCStatPlan(cfg, fp)
		if err == nil {
			return handle, nil
		}
		lastErr = err
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr,
				"ebpf-go.plugin: dcstat %s unavailable (%v), trying fallback\n",
				filepath.Base(fp.ObjectPath), err)
		}
	}
	return nil, lastErr
}

func LoadDCStatLegacyFromSystem() (*DCStatLegacyHandle, error) {
	cfg, err := resolveDCStatLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadDCStatLegacy(cfg)
}
