//go:build netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadSocketLegacy(cfg SocketLegacyConfig) (*SocketLegacyHandle, error) {
	plan := BuildSocketLegacyPlan(cfg)
	coreSupported := libbpfloader.SupportsCore()
	if !coreSupported {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		plan.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "socket", false, cfg.IsRHF, ObjectFlavorBase)
		plan.LoadMode = LoadLegacy
	}

	rt, err := libbpfloader.NewSocketRuntime(plan.ObjectPath, plan.LoadMode == LoadCore && coreSupported)
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

func LoadSocketLegacyFromSystem() (*SocketLegacyHandle, error) {
	cfg, err := resolveSocketLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadSocketLegacy(cfg)
}
