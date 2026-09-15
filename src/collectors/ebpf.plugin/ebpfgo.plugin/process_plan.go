package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

const processKernelMask uint32 = (1 << 12) - 1
const processDefaultPIDTableSize uint32 = 32768
const processMaxPIDTableSize uint32 = 32768
const processDefaultBTFFile = "vmlinux"

// processMaxBaseSelector is the highest SelectKernelName index for which a
// base-flavor (no suffix) process object file is shipped.
const processMaxBaseSelector = 11 // 6.12

type ProcessLegacyConfig struct {
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
	Targets        ProcessTargets
}

type ProcessLegacyHandle struct {
	Plan           LoadPlan
	Runtime        *libbpfloader.ProcessRuntime
	SharedMemory   *SharedPidMemoryPublisher
	UpdateEvery    int
	ConfigFound    bool
	PidTableSize   uint32
	MapsPerCore    bool
	AppsEnabled    bool
	CgroupsEnabled bool
	AppsLevel      int
}

func (h *ProcessLegacyHandle) Close() {
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

func defaultProcessLegacyConfig() ProcessLegacyConfig {
	return ProcessLegacyConfig{
		PluginsDir:     defaultPluginsDir(),
		Kernels:        processKernelMask,
		IsRHF:          -1,
		IsDebian:       IsDebianFlavor(),
		BTFPath:        processDefaultBTFPath,
		UpdateEvery:    processDefaultUpdateEvery,
		HasBTF:         kernelBTFSupported(processDefaultBTFPath),
		PidTableSize:   processDefaultPIDTableSize,
		MapsPerCore:    true,
		ObjectFlavor:   processDefaultObjectFlavor,
		Enabled:        true,
		AppsEnabled:    false,
		CgroupsEnabled: false,
		AppsLevel:      0, // NETDATA_APPS_LEVEL_REAL_PARENT — matches stock process.conf default
		Targets:        defaultProcessTargets(),
	}
}

func resolveProcessLegacyConfig() (ProcessLegacyConfig, error) {
	cfg := defaultProcessLegacyConfig()

	fileCfg, found, err := loadProcessConfigFiles()
	if err != nil {
		return ProcessLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.Process != nil {
		cfg.Enabled = *fileCfg.Process
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
		return ProcessLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	targets, err := resolveProcessTargets()
	if err != nil {
		return ProcessLegacyConfig{}, err
	}
	cfg.Targets = targets

	return cfg, nil
}

func BuildProcessLegacyPlan(cfg ProcessLegacyConfig) LoadPlan {
	flavor := cfg.ObjectFlavor
	// The buffer and arena process objects expose global counters and events,
	// but no tbl_pid_stats map. Apps/cgroups integration requires that map for
	// per-PID snapshots, so select the base object whenever those consumers are
	// enabled.
	if cfg.AppsEnabled || cfg.CgroupsEnabled {
		flavor = "legacy"
	}

	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:      cfg.PluginsDir,
		Kernels:         cfg.Kernels,
		IsRHF:           cfg.IsRHF,
		KernelVersion:   cfg.KernelVersion,
		IsDebian:        cfg.IsDebian,
		HasBTF:          cfg.HasBTF,
		ObjectFlavor:    flavor,
		Name:            "process",
		MaxBaseSelector: processMaxBaseSelector,
	})
}
