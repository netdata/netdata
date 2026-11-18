package main

import (
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// processKernelMask is the C process module's kernel list (3.10, 4.14, 4.16,
// 4.18, 5.4, 5.10, 5.14, 6.12, 7.0; SelectKernelName indexes): the kernels with
// a shipped base object.  Every flavor walks down to one of them.
const processKernelMask uint32 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<11 | 1<<12
const processDefaultPIDTableSize uint32 = 32768
const processDefaultBTFFile = "vmlinux"

// processMaxBaseSelector is the highest SelectKernelName index for which a
// base-flavor (no suffix) process object file is shipped.
const processMaxBaseSelector = 12 // 7.0

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
	// ReportErrors mirrors `ebpf load mode`: true for `return`/`dev`.  It gates
	// only the error charts; the return-probe objects always count task errors.
	ReportErrors bool
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
	ReportErrors   bool
}

func (h *ProcessLegacyHandle) Close() {
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
		Enabled:        false,
		AppsEnabled:    false,
		CgroupsEnabled: false,
		AppsLevel:      0,     // NETDATA_APPS_LEVEL_REAL_PARENT — matches stock process.conf default
		ReportErrors:   false, // `ebpf load mode = entry` is the stock default
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
		ReturnMode:     &cfg.ReportErrors,
	})
	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return ProcessLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

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
		// The `r`-prefixed objects count task-creation errors.  They are used in
		// both `entry` and `return` mode; see ProcessLegacyConfig.ReportErrors.
		IsReturn: true,
	})
}

// processCustomBTFPath returns the vmlinux BTF file from the configured
// directory: `btf path` is a directory option, while libbpf's open options take
// the file path.
func processCustomBTFPath(btfDir string) string {
	return filepath.Join(btfDir, processDefaultBTFFile)
}
