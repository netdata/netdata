package main

import (
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// fdKernelMask offers every selector index, as the other migrated collectors do.
// It is deliberately wider than the C module's `.kernels`
// (V3_10|V4_14|V4_16|V4_18|V5_4|V5_11|V5_14): the buffer and arena objects ship
// for kernels up to 6.12, so SelectIndex has to be able to reach selector 11.
// The base flavor is bounded by fdMaxBaseSelector instead, which is where the
// real object-availability limit lives.
const fdKernelMask uint32 = (1 << 12) - 1

const fdDefaultPIDTableSize uint32 = 32768

// fdMaxBaseSelector is the highest SelectKernelName index for which a
// base-flavor (no suffix) fd object file is shipped: 5.14.  Same ceiling as
// dcstat; cachestat goes one further (5.16).
//
// fd also ships no base object for selector 5 ("5.10"), unlike cachestat and
// dcstat.  No cap is needed for it: SelectMaxIndex never returns 5 — it jumps
// from 4 to 6 at the 5.10 boundary — so that selector is unreachable.  See
// TestSelectMaxIndexNeverReturnsTheMissingFDBaseSelector.
const fdMaxBaseSelector = 7 // 5.14

type FDLegacyConfig struct {
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
	// ReportErrors mirrors `ebpf load mode`: false for `entry` (the stock
	// default), true for `return`/`dev`.  It gates ONLY the error charts.  The
	// probes always read the syscall return value, because that is the only
	// variant the buffer and arena objects ship — the same situation the C module
	// was already in, where em->mode gated chart creation and never attachment.
	ReportErrors bool
	Targets      FDTargets
}

type FDLegacyHandle struct {
	Plan           LoadPlan
	Runtime        *libbpfloader.FDRuntime
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

func (h *FDLegacyHandle) Close() {
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

func defaultFDLegacyConfig() FDLegacyConfig {
	return FDLegacyConfig{
		PluginsDir:     defaultPluginsDir(),
		Kernels:        fdKernelMask,
		IsRHF:          -1,
		IsDebian:       IsDebianFlavor(),
		BTFPath:        fdDefaultBTFPath,
		UpdateEvery:    fdDefaultUpdateEvery,
		HasBTF:         kernelBTFSupported(fdDefaultBTFPath),
		PidTableSize:   fdDefaultPIDTableSize,
		MapsPerCore:    true,
		ObjectFlavor:   fdDefaultObjectFlavor,
		Enabled:        false,
		AppsEnabled:    false,
		CgroupsEnabled: false,
		AppsLevel:      0,     // NETDATA_APPS_LEVEL_REAL_PARENT — matches stock fd.conf default
		ReportErrors:   false, // `ebpf load mode = entry` is the stock default
	}
}

func resolveFDLegacyConfig() (FDLegacyConfig, error) {
	cfg := defaultFDLegacyConfig()

	fileCfg, found, err := loadFDConfigFiles()
	if err != nil {
		return FDLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.Fd != nil {
		cfg.Enabled = *fileCfg.Fd
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
		return FDLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	// Only touch /proc/kallsyms when fd will actually run: this function is
	// called at startup regardless of the module being enabled, and fd must not
	// influence the collectors sharing this process.  A resolution failure is
	// reported at load time (LoadFDLegacy), not here, so one module's missing
	// kernel symbol cannot abort plugin startup for the others.
	if cfg.Enabled {
		if targets, terr := resolveFDTargets(); terr == nil {
			cfg.Targets = targets
		} else {
			rateLimitedStderr("fd.kallsyms", "ebpf-go.plugin: %v\n", terr)
		}
	}

	return cfg, nil
}

func BuildFDLegacyPlan(cfg FDLegacyConfig) LoadPlan {
	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:      cfg.PluginsDir,
		Kernels:         cfg.Kernels,
		IsRHF:           cfg.IsRHF,
		KernelVersion:   cfg.KernelVersion,
		IsDebian:        cfg.IsDebian,
		HasBTF:          cfg.HasBTF,
		ObjectFlavor:    cfg.ObjectFlavor,
		Name:            "fd",
		MaxBaseSelector: fdMaxBaseSelector,
		// The `r`-prefixed objects are the ones whose programs read PT_REGS_RC.
		// They are used in both `entry` and `return` mode; see
		// FDLegacyConfig.ReportErrors.
		IsReturn: true,
	})
}
