package main

import (
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const dcstatKernelMask uint32 = (1 << 12) - 1
const dcstatDefaultPIDTableSize uint32 = 32768

// dcstatMaxBaseSelector is the highest SelectKernelName index for which a
// base-flavor (no suffix) dcstat object file is shipped.  Unlike cachestat
// (5.16) the dc family stops at 5.14, so the two constants differ.
const dcstatMaxBaseSelector = 7 // 5.14

type DCStatLegacyConfig struct {
	PluginsDir     string
	Kernels        uint32
	IsRHF          int
	KernelVersion  uint32
	IsDebian       bool
	HasBTF         bool
	ConfigFound    bool
	Enabled        bool
	AppsEnabled    bool
	CgroupsEnabled bool
	BTFPath        string
	UpdateEvery    int
	PidTableSize   uint32
	MapsPerCore    bool
	ObjectFlavor   string
	AppsLevel      int // BPF apps collection level: 0=real parent, 1=parent, 2=all
	Targets        DCStatTargets
}

type DCStatLegacyHandle struct {
	Plan           LoadPlan
	Runtime        *libbpfloader.DCStatRuntime
	SharedMemory   *SharedPidMemoryPublisher
	UpdateEvery    int
	ConfigFound    bool
	PidTableSize   uint32
	MapsPerCore    bool
	AppsEnabled    bool
	CgroupsEnabled bool
	AppsLevel      int
}

func (h *DCStatLegacyHandle) Close() {
	if h == nil {
		return
	}

	if h.Runtime != nil {
		h.Runtime.Close()
		h.Runtime = nil
	}
	if h.SharedMemory != nil {
		h.SharedMemory.Close()
		h.SharedMemory = nil
	}
}

func defaultDCStatLegacyConfig() DCStatLegacyConfig {
	return DCStatLegacyConfig{
		PluginsDir:     defaultPluginsDir(),
		Kernels:        dcstatKernelMask,
		IsRHF:          -1,
		IsDebian:       IsDebianFlavor(),
		BTFPath:        dcstatDefaultBTFPath,
		UpdateEvery:    dcstatDefaultUpdateEvery,
		HasBTF:         kernelBTFSupported(dcstatDefaultBTFPath),
		PidTableSize:   dcstatDefaultPIDTableSize,
		MapsPerCore:    true,
		ObjectFlavor:   dcstatDefaultObjectFlavor,
		Enabled:        false,
		AppsEnabled:    false,
		CgroupsEnabled: false,
		AppsLevel:      0, // NETDATA_APPS_LEVEL_REAL_PARENT — matches stock dcstat.conf default
		Targets:        defaultDCStatTargets(),
	}
}

func resolveDCStatLegacyConfig() (DCStatLegacyConfig, error) {
	cfg := defaultDCStatLegacyConfig()

	fileCfg, found, err := loadDCStatConfigFiles()
	if err != nil {
		return DCStatLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.Dcstat != nil {
		cfg.Enabled = *fileCfg.Dcstat
	}
	if fileCfg.UpdateEvery != nil && *fileCfg.UpdateEvery > 0 {
		cfg.UpdateEvery = *fileCfg.UpdateEvery
	}
	if fileCfg.AppsEnabled != nil {
		cfg.AppsEnabled = *fileCfg.AppsEnabled
	}
	if fileCfg.Cgroups != nil {
		cfg.CgroupsEnabled = *fileCfg.Cgroups
	}
	if fileCfg.PidTable != nil && *fileCfg.PidTable > 0 {
		cfg.PidTableSize = applyPidTableSizeClamp(*fileCfg.PidTable)
	}
	if fileCfg.MapsPerCore != nil {
		cfg.MapsPerCore = *fileCfg.MapsPerCore
	}
	if fileCfg.BTFPath != nil && *fileCfg.BTFPath != "" {
		cfg.BTFPath = *fileCfg.BTFPath
		cfg.HasBTF = kernelBTFSupported(cfg.BTFPath)
	}
	if fileCfg.CollectPidLevel != nil {
		cfg.AppsLevel = *fileCfg.CollectPidLevel
	}
	if fileCfg.ObjectFlavor != nil && *fileCfg.ObjectFlavor != "" {
		cfg.ObjectFlavor = *fileCfg.ObjectFlavor
	}
	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return DCStatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	targets, err := resolveDCStatTargets()
	if err != nil {
		return DCStatLegacyConfig{}, err
	}
	cfg.Targets = targets

	return cfg, nil
}

func BuildDCStatLegacyPlan(cfg DCStatLegacyConfig) LoadPlan {
	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:      cfg.PluginsDir,
		Kernels:         cfg.Kernels,
		IsRHF:           cfg.IsRHF,
		KernelVersion:   cfg.KernelVersion,
		IsDebian:        cfg.IsDebian,
		HasBTF:          cfg.HasBTF,
		ObjectFlavor:    cfg.ObjectFlavor,
		Name:            "dc",
		MaxBaseSelector: dcstatMaxBaseSelector,
	})
}
