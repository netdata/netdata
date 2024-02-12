// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/go.d.plugin/pkg/prometheus"
)

const (
	netframeworkPrefix = "netframework_"
)

const (
	metricNetFrameworkCLRExceptionsThrownTotal          = "windows_netframework_clrexceptions_exceptions_thrown_total"
	metricNetFrameworkCLRExceptionsFiltersTotal         = "windows_netframework_clrexceptions_exceptions_filters_total"
	metricNetFrameworkCLRExceptionsFinallysTotal        = "windows_netframework_clrexceptions_exceptions_finallys_total"
	metricNetFrameworkCLRExceptionsThrowCatchDepthTotal = "windows_netframework_clrexceptions_throw_to_catch_depth_total"
)

func (w *Windows) collectNetFrameworkCLRExceptions(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRExceptionsThrownTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrexception_thrown_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRExceptionsFiltersTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrexception_filters_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRExceptionsFinallysTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrexception_finallys_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRExceptionsThrowCatchDepthTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrexception_throw_to_catch_depth_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRExceptions[proc] {
			w.cache.netFrameworkCLRExceptions[proc] = true
			w.addProcessNetFrameworkExceptionsCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRExceptions {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRExceptions, proc)
			w.removeProcessFromNetFrameworkExceptionsCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRInteropComCallableWrappersTotal = "windows_netframework_clrinterop_com_callable_wrappers_total"
	metricNetFrameworkCLRInteropMarshallingTotal         = "windows_netframework_clrinterop_interop_marshalling_total"
	metricNetFrameworkCLRInteropStubsCreatedTotal        = "windows_netframework_clrinterop_interop_stubs_created_total"
)

func (w *Windows) collectNetFrameworkCLRInterop(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRInteropComCallableWrappersTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrinterop_com_callable_wrappers_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRInteropMarshallingTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrinterop_interop_marshalling_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRInteropStubsCreatedTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrinterop_interop_stubs_created_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRInterops[proc] {
			w.cache.netFrameworkCLRInterops[proc] = true
			w.addProcessNetFrameworkInteropCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRInterops {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRInterops, proc)
			w.removeProcessNetFrameworkInteropCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRJITMethodsTotal          = "windows_netframework_clrjit_jit_methods_total"
	metricNetFrameworkCLRJITTimePercent           = "windows_netframework_clrjit_jit_time_percent"
	metricNetFrameworkCLRJITStandardFailuresTotal = "windows_netframework_clrjit_jit_standard_failures_total"
	metricNetFrameworkCLRJITILBytesTotal          = "windows_netframework_clrjit_jit_il_bytes_total"
)

func (w *Windows) collectNetFrameworkCLRJIT(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRJITMethodsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrjit_methods_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRJITStandardFailuresTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrjit_standard_failures_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRJITTimePercent) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrjit_time_percent"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRJITILBytesTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrjit_il_bytes_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRJIT[proc] {
			w.cache.netFrameworkCLRJIT[proc] = true
			w.addProcessNetFrameworkJITCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRJIT {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRJIT, proc)
			w.removeProcessNetFrameworkJITCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRLoadingLoaderHeapSizeBytes    = "windows_netframework_clrloading_loader_heap_size_bytes"
	metricNetFrameworkCLRLoadingAppDomainLoadedTotal   = "windows_netframework_clrloading_appdomains_loaded_total"
	metricNetFrameworkCLRLoadingAppDomainUnloadedTotal = "windows_netframework_clrloading_appdomains_unloaded_total"
	metricNetFrameworkCLRLoadingAssembliesLoadedTotal  = "windows_netframework_clrloading_assemblies_loaded_total"
	metricNetFrameworkCLRLoadingClassesLoadedTotal     = "windows_netframework_clrloading_classes_loaded_total"
	metricNetFrameworkCLRLoadingClassLoadFailuresTotal = "windows_netframework_clrloading_class_load_failures_total"
)

func (w *Windows) collectNetFrameworkCLRLoading(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingLoaderHeapSizeBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_loader_heap_size_bytes"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingAppDomainLoadedTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_appdomains_loaded_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingAppDomainUnloadedTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_appdomains_unloaded_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingAssembliesLoadedTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_assemblies_loaded_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingClassesLoadedTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_classes_loaded_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLoadingClassLoadFailuresTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrloading_class_load_failures_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRLoading[proc] {
			w.cache.netFrameworkCLRLoading[proc] = true
			w.addProcessNetFrameworkLoadingCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRLoading {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRLoading, proc)
			w.removeProcessNetFrameworkLoadingCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRLocksAndThreadsQueueLengthTotal       = "windows_netframework_clrlocksandthreads_queue_length_total"
	metricNetFrameworkCLRLocksAndThreadsCurrentLogicalThreads  = "windows_netframework_clrlocksandthreads_current_logical_threads"
	metricNetFrameworkCLRLocksAndThreadsPhysicalThreadsCurrent = "windows_netframework_clrlocksandthreads_physical_threads_current"
	metricNetFrameworkCLRLocksAndThreadsRecognizedThreadsTotal = "windows_netframework_clrlocksandthreads_recognized_threads_total"
	metricNetFrameworkCLRLocksAndThreadsContentionsTotal       = "windows_netframework_clrlocksandthreads_contentions_total"
)

func (w *Windows) collectNetFrameworkCLRLocksAndThreads(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLocksAndThreadsQueueLengthTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrlocksandthreads_queue_length_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLocksAndThreadsCurrentLogicalThreads) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrlocksandthreads_current_logical_threads"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLocksAndThreadsPhysicalThreadsCurrent) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrlocksandthreads_physical_threads_current"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLocksAndThreadsRecognizedThreadsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrlocksandthreads_recognized_threads_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRLocksAndThreadsContentionsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrlocksandthreads_contentions_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRLocksThreads[proc] {
			w.cache.netFrameworkCLRLocksThreads[proc] = true
			w.addProcessNetFrameworkLocksAndThreadsCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRLocksThreads {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRLocksThreads, proc)
			w.removeProcessNetFrameworkLocksAndThreadsCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRMemoryAllocatedBytesTotal   = "windows_netframework_clrmemory_allocated_bytes_total"
	metricNetFrameworkCLRMemoryFinalizationSurvivors = "windows_netframework_clrmemory_finalization_survivors"
	metricNetFrameworkCLRMemoryHeapSizeBytes         = "windows_netframework_clrmemory_heap_size_bytes"
	metricNetFrameworkCLRMemoryPromotedBytes         = "windows_netframework_clrmemory_promoted_bytes"
	metricNetFrameworkCLRMemoryNumberGCHandles       = "windows_netframework_clrmemory_number_gc_handles"
	metricNetFrameworkCLRMemoryCollectionsTotal      = "windows_netframework_clrmemory_collections_total"
	metricNetFrameworkCLRMemoryInducedGCTotal        = "windows_netframework_clrmemory_induced_gc_total"
	metricNetFrameworkCLRMemoryNumberPinnedObjects   = "windows_netframework_clrmemory_number_pinned_objects"
	metricNetFrameworkCLRMemoryNumberSinkBlockInUse  = "windows_netframework_clrmemory_number_sink_blocksinuse"
	metricNetFrameworkCLRMemoryCommittedBytes        = "windows_netframework_clrmemory_committed_bytes"
	metricNetFrameworkCLRMemoryReservedBytes         = "windows_netframework_clrmemory_reserved_bytes"
	metricNetFrameworkCLRMemoryGCTimePercent         = "windows_netframework_clrmemory_gc_time_percent"
)

func (w *Windows) collectNetFrameworkCLRMemory(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryAllocatedBytesTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_allocated_bytes_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryFinalizationSurvivors) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_finalization_survivors"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryHeapSizeBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_heap_size_bytes"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryPromotedBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_promoted_bytes"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryNumberGCHandles) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_number_gc_handles"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryCollectionsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_collections_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryInducedGCTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_induced_gc_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryNumberPinnedObjects) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_number_pinned_objects"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryNumberSinkBlockInUse) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_number_sink_blocksinuse"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryCommittedBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_committed_bytes"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryReservedBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_reserved_bytes"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRMemoryGCTimePercent) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrmemory_gc_time_percent"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRMemory[proc] {
			w.cache.netFrameworkCLRMemory[proc] = true
			w.addProcessNetFrameworkMemoryCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRMemory {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRMemory, proc)
			w.removeProcessNetFrameworkMemoryCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRRemotingChannelsTotal             = "windows_netframework_clrremoting_channels_total"
	metricNetFrameworkCLRRemotingContextBoundClassesLoaded = "windows_netframework_clrremoting_context_bound_classes_loaded"
	metricNetFrameworkCLRRemotingContextBoundObjectsTotal  = "windows_netframework_clrremoting_context_bound_objects_total"
	metricNetFrameworkCLRRemotingContextProxiesTotal       = "windows_netframework_clrremoting_context_proxies_total"
	metricNetFrameworkCLRRemotingContexts                  = "windows_netframework_clrremoting_contexts"
	metricNetFrameworkCLRRemotingRemoteCallsTotal          = "windows_netframework_clrremoting_remote_calls_total"
)

func (w *Windows) collectNetFrameworkCLRRemoting(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingChannelsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_channels_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingContextBoundClassesLoaded) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_context_bound_classes_loaded"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingContextBoundObjectsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_context_bound_objects_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingContextProxiesTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_context_proxies_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingContexts) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_contexts"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRRemotingRemoteCallsTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrremoting_remote_calls_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRRemoting[proc] {
			w.cache.netFrameworkCLRRemoting[proc] = true
			w.addProcessNetFrameworkRemotingCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRRemoting {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRRemoting, proc)
			w.removeProcessNetFrameworkRemotingCharts(proc)
		}
	}
}

const (
	metricNetFrameworkCLRSecurityLinkTimeChecksTotal = "windows_netframework_clrsecurity_link_time_checks_total"
	metricNetFrameworkCLRSecurityRTChecksTimePercent = "windows_netframework_clrsecurity_rt_checks_time_percent"
	metricNetFrameworkCLRSecurityStackWalkDepth      = "windows_netframework_clrsecurity_stack_walk_depth"
	metricNetFrameworkCLRSecurityRuntimeChecksTotal  = "windows_netframework_clrsecurity_runtime_checks_total"
)

func (w *Windows) collectNetFrameworkCLRSecurity(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByName(metricNetFrameworkCLRSecurityLinkTimeChecksTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrsecurity_link_time_checks_total"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRSecurityRTChecksTimePercent) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrsecurity_checks_time_percent"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRSecurityStackWalkDepth) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrsecurity_stack_walk_depth"] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricNetFrameworkCLRSecurityRuntimeChecksTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[netframeworkPrefix+name+"_clrsecurity_runtime_checks_total"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !w.cache.netFrameworkCLRSecurity[proc] {
			w.cache.netFrameworkCLRSecurity[proc] = true
			w.addProcessNetFrameworkSecurityCharts(proc)
		}
	}

	for proc := range w.cache.netFrameworkCLRSecurity {
		if !seen[proc] {
			delete(w.cache.netFrameworkCLRSecurity, proc)
			w.removeProcessNetFrameworkSecurityCharts(proc)
		}
	}
}
