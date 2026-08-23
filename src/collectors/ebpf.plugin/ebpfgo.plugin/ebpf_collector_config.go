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
	AppsLevel      *int
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
	if fileCfg.ObjectFlavor != nil && *fileCfg.ObjectFlavor != "" && dst.ObjectFlavor != nil {
		*dst.ObjectFlavor = *fileCfg.ObjectFlavor
	}
}
