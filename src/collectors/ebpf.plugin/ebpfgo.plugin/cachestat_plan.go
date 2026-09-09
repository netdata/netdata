package main

import (
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const cachestatKernelMask uint32 = (1 << 12) - 1
const cachestatDefaultPIDTableSize uint32 = 32768
const cachestatMaxPIDTableSize uint32 = 32768
const cachestatDefaultBTFFile = "vmlinux"

// cachestatMaxBaseSelector is the highest SelectKernelName index for which a
// base-flavor (no suffix) cachestat object file is shipped.
const cachestatMaxBaseSelector = 9 // 5.16

type CachestatLegacyConfig struct {
	PluginsDir      string
	Kernels         uint32
	IsRHF           int
	KernelVersion   uint32
	IsDebian        bool
	HasBTF          bool
	ConfigFound     bool
	Enabled         bool
	AppsEnabled     bool
	CgroupsEnabled  bool
	BTFPath         string
	UpdateEvery     int
	PidTableSize    uint32
	MapsPerCore     bool
	ObjectFlavor    string
	AccountFunction string
	AppsLevel       int // BPF apps collection level: 0=real parent, 1=parent, 2=all
	Targets         CachestatTargets
}

type CachestatLegacyHandle struct {
	Plan           LoadPlan
	Runtime        *libbpfloader.CachestatRuntime
	SharedMemory   *SharedPidMemoryPublisher
	UpdateEvery    int
	ConfigFound    bool
	PidTableSize   uint32
	MapsPerCore    bool
	AppsEnabled    bool
	CgroupsEnabled bool
	AppsLevel      int
}

func (h *CachestatLegacyHandle) Close() {
	if h == nil || h.Runtime == nil {
		if h != nil && h.SharedMemory != nil {
			h.SharedMemory.Close()
			h.SharedMemory = nil
		}
		return
	}

	h.Runtime.Close()
	h.Runtime = nil
	if h.SharedMemory != nil {
		h.SharedMemory.Close()
		h.SharedMemory = nil
	}
}

func defaultPluginsDir() string {
	if dir := os.Getenv("NETDATA_PLUGINS_DIR"); dir != "" {
		return dir
	}

	return filepath.Join(netdataRuntimePrefix, "usr/libexec/netdata/plugins.d")
}

func defaultCachestatLegacyConfig() CachestatLegacyConfig {
	return CachestatLegacyConfig{
		PluginsDir:     defaultPluginsDir(),
		Kernels:        cachestatKernelMask,
		IsRHF:          -1,
		IsDebian:       IsDebianFlavor(),
		BTFPath:        cachestatDefaultBTFPath,
		UpdateEvery:    cachestatDefaultUpdateEvery,
		HasBTF:         kernelBTFSupported(cachestatDefaultBTFPath),
		PidTableSize:   cachestatDefaultPIDTableSize,
		MapsPerCore:    true,
		ObjectFlavor:   cachestatDefaultObjectFlavor,
		Enabled:        true,
		AppsEnabled:    false,
		CgroupsEnabled: false,
		AppsLevel:      0, // NETDATA_APPS_LEVEL_REAL_PARENT — matches stock cachestat.conf default
		Targets:        defaultCachestatTargets(),
	}
}

func resolveCachestatLegacyConfig() (CachestatLegacyConfig, error) {
	cfg := defaultCachestatLegacyConfig()

	fileCfg, found, err := loadCachestatConfigFiles()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.Cachestat != nil {
		cfg.Enabled = *fileCfg.Cachestat
	}
	applyCommonCollectorConfig(fileCfg, collectorCommonConfig{
		UpdateEvery:    &cfg.UpdateEvery,
		AppsEnabled:    &cfg.AppsEnabled,
		CgroupsEnabled: &cfg.CgroupsEnabled,
		PidTableSize:   &cfg.PidTableSize,
		MapsPerCore:    &cfg.MapsPerCore,
		BTFPath:        &cfg.BTFPath,
		HasBTF:         &cfg.HasBTF,
		ObjectFlavor:   &cfg.ObjectFlavor,
		AppsLevel:      &cfg.AppsLevel,
	})
	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	if err := cfg.Targets.ResolveAccountPageTarget(); err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.AccountFunction = cfg.Targets.AccountPageDirtied.Name

	return cfg, nil
}

func BuildCachestatLegacyPlan(cfg CachestatLegacyConfig) LoadPlan {
	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:      cfg.PluginsDir,
		Kernels:         cfg.Kernels,
		IsRHF:           cfg.IsRHF,
		KernelVersion:   cfg.KernelVersion,
		IsDebian:        cfg.IsDebian,
		HasBTF:          cfg.HasBTF,
		ObjectFlavor:    cfg.ObjectFlavor,
		Name:            "cachestat",
		MaxBaseSelector: cachestatMaxBaseSelector,
	})
}

func kernelBTFSupported(btfPath string) bool {
	_, err := os.Stat(filepath.Join(btfPath, cachestatDefaultBTFFile))
	return err == nil
}

func applyPidTableSizeClamp(n uint32) uint32 {
	if n > cachestatMaxPIDTableSize {
		return cachestatMaxPIDTableSize
	}
	return n
}
