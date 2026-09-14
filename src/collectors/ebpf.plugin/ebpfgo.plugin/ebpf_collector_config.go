package main

// collectorCommonConfig points at the fields every kprobe-based collector
// resolves from the shared `[global]` config section.  Each collector keeps its
// own config struct (their remaining fields differ), and passes the addresses of
// the shared ones here so the merge rules live in one place.
type collectorCommonConfig struct {
	UpdateEvery    *int
	AppsEnabled    *bool
	CgroupsEnabled *bool
	PidTableSize   *uint32
	MapsPerCore    *bool
	BTFPath        *string
	HasBTF         *bool
	ObjectFlavor   *string
	LoadMethod     *LoadMethod
	AppsLevel      *int
	// ReturnMode receives `ebpf load mode`.  Only fd passes it: it is the only
	// module whose charts differ between `entry` and `return`.
	ReturnMode *bool
}

// applyCommonCollectorConfig merges the `[global]` keys onto a collector's
// config.  A key absent from every config file leaves the collector's default
// untouched, which is why pluginConfigFile uses pointers.
//
// `lifetime` is parsed but intentionally unused: it bounds how long a thread
// runs when activated by a cloud Function call in the old ebpf.plugin, and the
// Go plugin runs until signalled instead.
func applyCommonCollectorConfig(fileCfg pluginConfigFile, dst collectorCommonConfig) {
	if fileCfg.UpdateEvery != nil && *fileCfg.UpdateEvery > 0 && dst.UpdateEvery != nil {
		*dst.UpdateEvery = *fileCfg.UpdateEvery
	}
	if fileCfg.AppsEnabled != nil && dst.AppsEnabled != nil {
		*dst.AppsEnabled = *fileCfg.AppsEnabled
	}
	if fileCfg.Cgroups != nil && dst.CgroupsEnabled != nil {
		*dst.CgroupsEnabled = *fileCfg.Cgroups
	}
	if fileCfg.PidTable != nil && *fileCfg.PidTable > 0 && dst.PidTableSize != nil {
		*dst.PidTableSize = applyPidTableSizeClamp(*fileCfg.PidTable)
	}
	if fileCfg.MapsPerCore != nil && dst.MapsPerCore != nil {
		*dst.MapsPerCore = *fileCfg.MapsPerCore
	}
	if fileCfg.BTFPath != nil && *fileCfg.BTFPath != "" && dst.BTFPath != nil {
		*dst.BTFPath = *fileCfg.BTFPath
		if dst.HasBTF != nil {
			*dst.HasBTF = kernelBTFSupported(*dst.BTFPath)
		}
	}
	if fileCfg.CollectPidLevel != nil && dst.AppsLevel != nil {
		*dst.AppsLevel = *fileCfg.CollectPidLevel
	}
	// `ebpf type format` and `ebpf object flavor` both want to decide which object
	// is loaded, so an explicit type format wins — but ONLY for a collector that
	// actually consumes the load method.
	//
	// fd is currently the only one (it derives the base flavor from LoadLegacy in
	// BuildFDLegacyPlan), so for fd the flavor merge is suppressed and the
	// operator's `ebpf object flavor` is left untouched rather than clobbered with
	// the legacy marker.  cachestat, dcstat and socket pass no LoadMethod
	// destination; for them the legacy marker must still reach ObjectFlavor, or
	// `ebpf type format = legacy` silently selects nothing at all.
	loadMethodApplied := false
	if fileCfg.LoadMethod != nil && *fileCfg.LoadMethod != LoadPlayDice && dst.LoadMethod != nil {
		*dst.LoadMethod = *fileCfg.LoadMethod
		loadMethodApplied = true
	}
	legacyMethodApplied := loadMethodApplied && *fileCfg.LoadMethod == LoadLegacy
	if !legacyMethodApplied && fileCfg.ObjectFlavor != nil && *fileCfg.ObjectFlavor != "" && dst.ObjectFlavor != nil {
		*dst.ObjectFlavor = *fileCfg.ObjectFlavor
	}
	if fileCfg.LoadModeReturn != nil && dst.ReturnMode != nil {
		*dst.ReturnMode = *fileCfg.LoadModeReturn
	}
}
