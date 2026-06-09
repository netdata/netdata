//go:build netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadCachestatLegacy(cfg CachestatLegacyConfig) (*CachestatLegacyHandle, error) {
	plan := BuildCachestatLegacyPlan(cfg)
	coreSupported := libbpfloader.SupportsCore()
	if !coreSupported {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		plan.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "cachestat", false, cfg.IsRHF, ObjectFlavorBase)
		plan.LoadMode = LoadLegacy
	}

	rt, err := libbpfloader.NewCachestatRuntime(plan.ObjectPath, plan.LoadMode == LoadCore && coreSupported)
	if err != nil {
		return nil, err
	}

	if err := rt.Prepare(cfg.PidTableSize, cfg.MapsPerCore, cfg.AccountFunction); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Attach(cfg.AccountFunction); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.UpdateController(cfg.AppsEnabled || cfg.CgroupsEnabled, cfg.AppsLevel); err != nil {
		rt.Close()
		return nil, err
	}

	// The shared memory publisher is opened lazily on the first publish call
	// (see runCachestatPlugin).  Opening here unconditionally would reserve
	// ~17.5 MB of VMA and RSS for hosts that have neither apps nor cgroups
	// integration enabled (the default), even though the SHM is never
	// written or read.  The handle exposes the PidTableSize to the lazy
	// open path.

	return &CachestatLegacyHandle{
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

func LoadCachestatLegacyFromSystem() (*CachestatLegacyHandle, error) {
	cfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadCachestatLegacy(cfg)
}
