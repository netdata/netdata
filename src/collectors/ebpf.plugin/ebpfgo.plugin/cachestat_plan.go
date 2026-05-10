package main

import (
	"os"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const cachestatKernelMask uint32 = (1 << 12) - 1

type CachestatLegacyConfig struct {
	PluginsDir    string
	Kernels       uint32
	IsRHF         int
	KernelVersion uint32
	IsDebian      bool
}

type CachestatLegacyHandle struct {
	Plan   LoadPlan
	Object *libbpfloader.Object
}

func (h *CachestatLegacyHandle) Close() {
	if h == nil || h.Object == nil {
		return
	}

	h.Object.Close()
	h.Object = nil
}

func defaultPluginsDir() string {
	if dir := os.Getenv("NETDATA_PLUGINS_DIR"); dir != "" {
		return dir
	}

	return "/usr/libexec/netdata/plugins.d"
}

func defaultCachestatLegacyConfig() CachestatLegacyConfig {
	return CachestatLegacyConfig{
		PluginsDir: defaultPluginsDir(),
		Kernels:    cachestatKernelMask,
		IsRHF:      -1,
		IsDebian:   IsDebianFlavor(),
	}
}

func resolveCachestatLegacyConfig() (CachestatLegacyConfig, error) {
	cfg := defaultCachestatLegacyConfig()

	kver, err := KernelVersion()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver

	if rhf, err := RedHatRelease(); err == nil {
		cfg.IsRHF = rhf
	}

	return cfg, nil
}

func BuildCachestatLegacyPlan(cfg CachestatLegacyConfig) LoadPlan {
	return BuildLoadPlan(
		cfg.PluginsDir,
		cfg.Kernels,
		cfg.IsRHF,
		cfg.KernelVersion,
		"cachestat",
		false,
		true,
		cfg.IsDebian,
		false,
		LoadLegacy,
		"",
		RunModeEntry,
	)
}
