package main

import (
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const cachestatKernelMask uint32 = (1 << 12) - 1
const cachestatDefaultPIDTableSize uint32 = 32768
const cachestatDefaultBTFFile = "vmlinux"

type CachestatLegacyConfig struct {
	PluginsDir      string
	Kernels         uint32
	IsRHF           int
	KernelVersion   uint32
	IsDebian        bool
	HasBTF          bool
	BTFPath         string
	UpdateEvery     int
	PidTableSize    uint32
	MapsPerCore     bool
	AccountFunction string
	Targets         CachestatTargets
}

type CachestatLegacyHandle struct {
	Plan        LoadPlan
	Runtime     *libbpfloader.CachestatRuntime
	UpdateEvery int
}

func (h *CachestatLegacyHandle) Close() {
	if h == nil || h.Runtime == nil {
		return
	}

	h.Runtime.Close()
	h.Runtime = nil
}

func defaultPluginsDir() string {
	if dir := os.Getenv("NETDATA_PLUGINS_DIR"); dir != "" {
		return dir
	}

	return filepath.Join(netdataRuntimePrefix, "usr/libexec/netdata/plugins.d")
}

func defaultCachestatLegacyConfig() CachestatLegacyConfig {
	return CachestatLegacyConfig{
		PluginsDir:   defaultPluginsDir(),
		Kernels:      cachestatKernelMask,
		IsRHF:        -1,
		IsDebian:     IsDebianFlavor(),
		BTFPath:      cachestatDefaultBTFPath,
		UpdateEvery:  cachestatDefaultUpdateEvery,
		HasBTF:       kernelBTFSupported(cachestatDefaultBTFPath),
		PidTableSize: cachestatDefaultPIDTableSize,
		MapsPerCore:  true,
		Targets:      defaultCachestatTargets(),
	}
}

func resolveCachestatLegacyConfig() (CachestatLegacyConfig, error) {
	cfg := defaultCachestatLegacyConfig()

	fileCfg, err := loadCachestatConfigFiles()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	if fileCfg.UpdateEvery != nil && *fileCfg.UpdateEvery > 0 {
		cfg.UpdateEvery = *fileCfg.UpdateEvery
	}
	if fileCfg.PidTable != nil && *fileCfg.PidTable > 0 {
		cfg.PidTableSize = *fileCfg.PidTable
	}
	if fileCfg.MapsPerCore != nil {
		cfg.MapsPerCore = *fileCfg.MapsPerCore
	}
	if fileCfg.BTFPath != nil && *fileCfg.BTFPath != "" {
		cfg.BTFPath = *fileCfg.BTFPath
		cfg.HasBTF = kernelBTFSupported(cfg.BTFPath)
	}
	if fileCfg.Lifetime != nil && *fileCfg.Lifetime > 0 {
		// Keep the legacy lifetime value available for future runtime wiring.
		// The current cachestat migration does not consume it yet.
	}

	kver, err := KernelVersion()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver

	if rhf, err := RedHatRelease(); err == nil {
		cfg.IsRHF = rhf
	}

	if err := cfg.Targets.ResolveAccountPageTarget(); err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.AccountFunction = cfg.Targets.AccountPageDirtied.Name

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
		cfg.HasBTF,
		LoadCore,
		"",
		RunModeEntry,
	)
}

func kernelBTFSupported(btfPath string) bool {
	_, err := os.Stat(filepath.Join(btfPath, cachestatDefaultBTFFile))
	return err == nil
}
