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
		return DCStatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	// Only touch /proc/kallsyms when dcstat will actually run: this function is
	// called at startup regardless of the module being enabled, and dcstat must
	// not influence the collectors sharing this process.
	if cfg.Enabled {
		cfg.Targets = resolveDCStatTargets()
	}

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
