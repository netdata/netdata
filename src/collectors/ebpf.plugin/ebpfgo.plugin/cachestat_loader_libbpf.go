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

	if err := rt.UpdateController(cfg.AppsEnabled || cfg.CgroupsEnabled); err != nil {
		rt.Close()
		return nil, err
	}

	publisher, err := NewSharedPidMemoryPublisher(cfg.PidTableSize)
	if err != nil {
		rt.Close()
		return nil, err
	}

	return &CachestatLegacyHandle{
		Plan:           plan,
		Runtime:        rt,
		SharedMemory:   publisher,
		UpdateEvery:    cfg.UpdateEvery,
		ConfigFound:    cfg.ConfigFound,
		PidTableSize:   cfg.PidTableSize,
		AppsEnabled:    cfg.AppsEnabled,
		CgroupsEnabled: cfg.CgroupsEnabled,
	}, nil
}

func LoadCachestatLegacyFromSystem() (*CachestatLegacyHandle, error) {
	cfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadCachestatLegacy(cfg)
}
