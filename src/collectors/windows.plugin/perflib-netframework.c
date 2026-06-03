// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

enum netdata_netframework_metrics {
    NETDATA_NETFRAMEWORK_EXCEPTIONS,
    NETDATA_NETFRAMEWORK_INTEROP,
    NETDATA_NETFRAMEWORK_JIT,
    NETDATA_NETFRAMEWORK_LOADING,
    NETDATA_NETFRAMEWORK_MEMORY,
    NETDATA_NETFRAMEWORK_REMOTING,
    NETDATA_NETFRAMEWORK_SECURITY,
    NETDATA_NETFRAMEWORK_LOCKS_THREADS,

    NETDATA_NETFRAMEWORK_END
};

struct net_framework_instances {
    usec_t last_collected;

    RRDSET *st_clrexception_thrown;
    RRDDIM *rd_clrexception_thrown;

    RRDSET *st_clrexception_filters;
    RRDDIM *rd_clrexception_filters;

    RRDSET *st_clrexception_finallys;
    RRDDIM *rd_clrexception_finallys;

    RRDSET *st_clrexception_total_catch_depth;
    RRDDIM *rd_clrexception_total_catch_depth;

    RRDSET *st_clrinterop_com_callable_wrappers;
    RRDDIM *rd_clrinterop_com_callable_wrappers;

    RRDSET *st_clrinterop_marshalling;
    RRDDIM *rd_clrinterop_marshalling;

    RRDSET *st_clrinterop_interop_stubs_created;
    RRDDIM *rd_clrinterop_interop_stubs_created;

    RRDSET *st_clrjit_methods;
    RRDDIM *rd_clrjit_methods;

    RRDSET *st_clrjit_time;
    RRDDIM *rd_clrjit_time;

    RRDSET *st_clrjit_standard_failures;
    RRDDIM *rd_clrjit_standard_failures;

    RRDSET *st_clrjit_il_bytes;
    RRDDIM *rd_clrjit_il_bytes;

    RRDSET *st_clrloading_heap_size;
    RRDDIM *rd_clrloading_heap_size;

    RRDSET *st_clrloading_current_appdomains;
    RRDDIM *rd_clrloading_current_appdomains;

    RRDSET *st_clrloading_current_assemblies;
    RRDDIM *rd_clrloading_current_assemblies;

    RRDSET *st_clrloading_current_classes_loaded;
    RRDDIM *rd_clrloading_current_classes_loaded;

    RRDSET *st_clrloading_app_domains_loaded;
    RRDDIM *rd_clrloading_app_domains_loaded;

    RRDSET *st_clrloading_app_domains_unloaded;
    RRDDIM *rd_clrloading_app_domains_unloaded;

    RRDSET *st_clrloading_assemblies_loaded;
    RRDDIM *rd_clrloading_assemblies_loaded;

    RRDSET *st_clrloading_classes_loaded;
    RRDDIM *rd_clrloading_classes_loaded;

    RRDSET *st_clrloading_class_load_failure;
    RRDDIM *rd_clrloading_class_load_failure;

    RRDSET *st_clrmemory_allocated_bytes;
    RRDDIM *rd_clrmemory_allocated_bytes;

    RRDSET *st_clrmemory_finalization_survivors;
    RRDDIM *rd_clrmemory_finalization_survivors;

    RRDSET *st_clrmemory_heap_size;
    RRDDIM *rd_clrmemory_heap_gen0;
    RRDDIM *rd_clrmemory_heap_gen1;
    RRDDIM *rd_clrmemory_heap_gen2;
    RRDDIM *rd_clrmemory_heap_loh;

    RRDSET *st_clrmemory_promoted_bytes;
    RRDDIM *rd_clrmemory_promoted_gen0;
    RRDDIM *rd_clrmemory_promoted_gen1;

    RRDSET *st_clrmemory_gc_handles;
    RRDDIM *rd_clrmemory_gc_handles;

    RRDSET *st_clrmemory_collections;
    RRDDIM *rd_clrmemory_collections_gen0;
    RRDDIM *rd_clrmemory_collections_gen1;
    RRDDIM *rd_clrmemory_collections_gen2;

    RRDSET *st_clrmemory_induced_gc;
    RRDDIM *rd_clrmemory_induced_gc;

    RRDSET *st_clrmemory_pinned_objects;
    RRDDIM *rd_clrmemory_pinned_objects;

    RRDSET *st_clrmemory_sink_blocks_in_use;
    RRDDIM *rd_clrmemory_sink_blocks_in_use;

    RRDSET *st_clrmemory_committed_bytes;
    RRDDIM *rd_clrmemory_committed_bytes;

    RRDSET *st_clrmemory_reserved_bytes;
    RRDDIM *rd_clrmemory_reserved_bytes;

    RRDSET *st_clrmemory_gc_time;
    RRDDIM *rd_clrmemory_gc_time;

    RRDSET *st_clrremoting_channels;
    RRDDIM *rd_clrremoting_channels;

    RRDSET *st_clrremoting_context_bound_classes_loaded;
    RRDDIM *rd_clrremoting_context_bound_classes_loaded;

    RRDSET *st_clrremoting_context_bound_objects;
    RRDDIM *rd_clrremoting_context_bound_objects;

    RRDSET *st_clrremoting_context_proxies;
    RRDDIM *rd_clrremoting_context_proxies;

    RRDSET *st_clrremoting_contexts;
    RRDDIM *rd_clrremoting_contexts;

    RRDSET *st_clrremoting_remote_calls;
    RRDDIM *rd_clrremoting_remote_calls;

    RRDSET *st_clrsecurity_link_time_checks;
    RRDDIM *rd_clrsecurity_link_time_checks;

    RRDSET *st_clrsecurity_rt_checks_time;
    RRDDIM *rd_clrsecurity_rt_checks_time;

    RRDSET *st_clrsecurity_stack_walk_depth;
    RRDDIM *rd_clrsecurity_stack_walk_depth;

    RRDSET *st_clrsecurity_run_time_checks;
    RRDDIM *rd_clrsecurity_run_time_checks;

    RRDSET *st_clrlocksandthreads_queue_length;
    RRDDIM *rd_locksandthreads_queue_length;

    RRDSET *st_clrlocksandthreads_current_queue_length;
    RRDDIM *rd_locksandthreads_current_queue_length;

    RRDSET *st_clrlocksandthreads_current_logical_threads;
    RRDDIM *rd_locksandthreads_current_logical_threads;

    RRDSET *st_clrlocksandthreads_current_physical_threads;
    RRDDIM *rd_locksandthreads_current_physical_threads;

    RRDSET *st_clrlocksandthreads_recognized_threads;
    RRDDIM *rd_locksandthreads_recognized_threads;

    RRDSET *st_clrlocksandthreads_current_recognized_threads;
    RRDDIM *rd_locksandthreads_current_recognized_threads;

    RRDSET *st_clrlocksandthreads_queue_length_peak;
    RRDDIM *rd_locksandthreads_queue_length_peak;

    RRDSET *st_clrlocksandthreads_contentions;
    RRDDIM *rd_locksandthreads_contentions;

    COUNTER_DATA NETFrameworkCLRExceptionThrown;
    COUNTER_DATA NETFrameworkCLRExceptionFilters;
    COUNTER_DATA NETFrameworkCLRExceptionFinallys;
    COUNTER_DATA NETFrameworkCLRExceptionTotalCatchDepth;

    COUNTER_DATA NETFrameworkCLRInteropCOMCallableWrappers;
    COUNTER_DATA NETFrameworkCLRInteropMarshalling;
    COUNTER_DATA NETFrameworkCLRInteropStubsCreated;

    COUNTER_DATA NETFrameworkCLRJITMethods;
    COUNTER_DATA NETFrameworkCLRJITPercentTime;
    COUNTER_DATA NETFrameworkCLRJITFrequencyTime;
    COUNTER_DATA NETFrameworkCLRJITStandardFailures;
    COUNTER_DATA NETFrameworkCLRJITIlBytes;

    COUNTER_DATA NETFrameworkCLRLoadingHeapSize;
    COUNTER_DATA NETFrameworkCLRLoadingCurrentAppdomains;
    COUNTER_DATA NETFrameworkCLRLoadingCurrentAssemblies;
    COUNTER_DATA NETFrameworkCLRLoadingCurrentClassesLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingAppDomainsLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingAppDomainsUnloaded;
    COUNTER_DATA NETFrameworkCLRLoadingAssembliesLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingClassesLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingClassLoadFailure;

    COUNTER_DATA NETFrameworkCLRMemoryAllocatedBytesPerSec;
    COUNTER_DATA NETFrameworkCLRMemoryFinalizationSurvivors;
    COUNTER_DATA NETFrameworkCLRMemoryGen0HeapSize;
    COUNTER_DATA NETFrameworkCLRMemoryGen0PromotedBytesPerSec;
    COUNTER_DATA NETFrameworkCLRMemoryGen1HeapSize;
    COUNTER_DATA NETFrameworkCLRMemoryGen1PromotedBytesPerSec;
    COUNTER_DATA NETFrameworkCLRMemoryGen2HeapSize;
    COUNTER_DATA NETFrameworkCLRMemoryLargeObjectHeapSize;
    COUNTER_DATA NETFrameworkCLRMemoryGCHandles;
    COUNTER_DATA NETFrameworkCLRMemoryGen0Collections;
    COUNTER_DATA NETFrameworkCLRMemoryGen1Collections;
    COUNTER_DATA NETFrameworkCLRMemoryGen2Collections;
    COUNTER_DATA NETFrameworkCLRMemoryInducedGC;
    COUNTER_DATA NETFrameworkCLRMemoryPinnedObjects;
    COUNTER_DATA NETFrameworkCLRMemorySinkBlocksInUse;
    COUNTER_DATA NETFrameworkCLRMemoryCommittedBytes;
    COUNTER_DATA NETFrameworkCLRMemoryReservedBytes;
    COUNTER_DATA NETFrameworkCLRMemoryTimeInGC;

    COUNTER_DATA NETFrameworkCLRRemotingChannels;
    COUNTER_DATA NETFrameworkCLRRemotingContextBoundClassesLoaded;
    COUNTER_DATA NETFrameworkCLRRemotingContextBoundObjects;
    COUNTER_DATA NETFrameworkCLRRemotingContextProxies;
    COUNTER_DATA NETFrameworkCLRRemotingContexts;
    COUNTER_DATA NETFrameworkCLRRemotingRemoteCalls;

    COUNTER_DATA NETFrameworkCLRSecurityLinkTimeChecks;
    COUNTER_DATA NETFrameworkCLRSecurityPercentTimeinRTChecks;
    COUNTER_DATA NETFrameworkCLRSecurityFrequency_PerfTime;
    COUNTER_DATA NETFrameworkCLRSecurityStackWalkDepth;
    COUNTER_DATA NETFrameworkCLRSecurityRunTimeChecks;

    COUNTER_DATA NETFrameworkCLRLocksAndThreadsQueueLength;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsCurrentQueueLength;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsCurrentLogicalThreads;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsRecognizedThreads;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsCurrentRecognizedThreads;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsQueueLengthPeak;
    COUNTER_DATA NETFrameworkCLRLocksAndThreadsContentions;
};

static usec_t netframework_now_ut = 0;
static DICTIONARY *processes = NULL;

static inline void initialize_net_framework_processes_keys(struct net_framework_instances *p)
{
    p->NETFrameworkCLRExceptionFilters.key = "# of Filters / sec";
    p->NETFrameworkCLRExceptionFinallys.key = "# of Finallys / sec";
    p->NETFrameworkCLRExceptionThrown.key = "# of Exceps Thrown / sec";
    p->NETFrameworkCLRExceptionTotalCatchDepth.key = "Throw To Catch Depth / sec";

    p->NETFrameworkCLRInteropCOMCallableWrappers.key = "# of CCWs";
    p->NETFrameworkCLRInteropMarshalling.key = "# of Stubs";
    p->NETFrameworkCLRInteropStubsCreated.key = "# of marshalling";

    p->NETFrameworkCLRJITMethods.key = "# of Methods Jitted";
    p->NETFrameworkCLRJITPercentTime.key = "% Time in Jit";
    p->NETFrameworkCLRJITFrequencyTime.key = "IL Bytes Jitted / sec";
    p->NETFrameworkCLRJITStandardFailures.key = "Standard Jit Failures";
    p->NETFrameworkCLRJITIlBytes.key = "# of IL Bytes Jitted";

    p->NETFrameworkCLRLoadingHeapSize.key = "Bytes in Loader Heap";
    p->NETFrameworkCLRLoadingCurrentAppdomains.key = "Current appdomains";
    p->NETFrameworkCLRLoadingCurrentAssemblies.key = "Current Assemblies";
    p->NETFrameworkCLRLoadingCurrentClassesLoaded.key = "Current Classes Loaded";
    p->NETFrameworkCLRLoadingAppDomainsLoaded.key = "Rate of appdomains";
    p->NETFrameworkCLRLoadingAppDomainsUnloaded.key = "Total appdomains unloaded";
    p->NETFrameworkCLRLoadingAssembliesLoaded.key = "Total Assemblies";
    p->NETFrameworkCLRLoadingClassesLoaded.key = "Total Classes Loaded";
    p->NETFrameworkCLRLoadingClassLoadFailure.key = "Total # of Load Failures";

    p->NETFrameworkCLRMemoryAllocatedBytesPerSec.key = "Allocated Bytes/sec";
    p->NETFrameworkCLRMemoryFinalizationSurvivors.key = "Finalization Survivors";
    p->NETFrameworkCLRMemoryGen0HeapSize.key = "Gen 0 heap size";
    p->NETFrameworkCLRMemoryGen0PromotedBytesPerSec.key = "Gen 0 Promoted Bytes/Sec";
    p->NETFrameworkCLRMemoryGen1HeapSize.key = "Gen 1 heap size";
    p->NETFrameworkCLRMemoryGen1PromotedBytesPerSec.key = "Gen 1 Promoted Bytes/Sec";
    p->NETFrameworkCLRMemoryGen2HeapSize.key = "Gen 2 heap size";
    p->NETFrameworkCLRMemoryLargeObjectHeapSize.key = "Large Object Heap size";
    p->NETFrameworkCLRMemoryGCHandles.key = "# GC Handles";
    p->NETFrameworkCLRMemoryGen0Collections.key = "# Gen 0 Collections";
    p->NETFrameworkCLRMemoryGen1Collections.key = "# Gen 1 Collections";
    p->NETFrameworkCLRMemoryGen2Collections.key = "# Gen 2 Collections";
    p->NETFrameworkCLRMemoryInducedGC.key = "# Induced GC";
    p->NETFrameworkCLRMemoryPinnedObjects.key = "# of Pinned Objects";
    p->NETFrameworkCLRMemorySinkBlocksInUse.key = "# of Sink Blocks in use";
    p->NETFrameworkCLRMemoryCommittedBytes.key = "# Total committed Bytes";
    p->NETFrameworkCLRMemoryReservedBytes.key = "# Total reserved Bytes";
    p->NETFrameworkCLRMemoryTimeInGC.key = "% Time in GC";

    p->NETFrameworkCLRRemotingChannels.key = "Channels";
    p->NETFrameworkCLRRemotingContextBoundClassesLoaded.key = "Context-Bound Classes Loaded";
    p->NETFrameworkCLRRemotingContextBoundObjects.key = "Context-Bound Objects Alloc / sec";
    p->NETFrameworkCLRRemotingContextProxies.key = "Context Proxies";
    p->NETFrameworkCLRRemotingContexts.key = "Contexts";
    p->NETFrameworkCLRRemotingRemoteCalls.key = "Total Remote Calls";

    p->NETFrameworkCLRSecurityLinkTimeChecks.key = "# Link Time Checks";
    p->NETFrameworkCLRSecurityPercentTimeinRTChecks.key = "% Time Sig. Authenticating";
    p->NETFrameworkCLRSecurityFrequency_PerfTime.key = "% Time in RT checks";
    p->NETFrameworkCLRSecurityStackWalkDepth.key = "Stack Walk Depth";
    p->NETFrameworkCLRSecurityRunTimeChecks.key = "Total Runtime Checks";

    p->NETFrameworkCLRLocksAndThreadsQueueLength.key = "Queue Length / sec";
    p->NETFrameworkCLRLocksAndThreadsCurrentQueueLength.key = "Current Queue Length";
    p->NETFrameworkCLRLocksAndThreadsCurrentLogicalThreads.key = "# of current logical Threads";
    p->NETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads.key = "# of current physical Threads";
    p->NETFrameworkCLRLocksAndThreadsRecognizedThreads.key = "# of total recognized threads";
    p->NETFrameworkCLRLocksAndThreadsCurrentRecognizedThreads.key = "# of current recognized threads";
    p->NETFrameworkCLRLocksAndThreadsQueueLengthPeak.key = "Queue Length Peak";
    p->NETFrameworkCLRLocksAndThreadsContentions.key = "Total # of Contentions";
}

void dict_net_framework_processes_insert_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    struct net_framework_instances *p = value;
    initialize_net_framework_processes_keys(p);
}

static void netframework_mark_chart_obsolete(RRDSET **st)
{
    if (*st)
        rrdset_is_obsolete___safe_from_collector_thread(*st);

    *st = NULL;
}

static void netframework_process_cleanup(struct net_framework_instances *p)
{
    netframework_mark_chart_obsolete(&p->st_clrexception_thrown);
    netframework_mark_chart_obsolete(&p->st_clrexception_filters);
    netframework_mark_chart_obsolete(&p->st_clrexception_finallys);
    netframework_mark_chart_obsolete(&p->st_clrexception_total_catch_depth);
    netframework_mark_chart_obsolete(&p->st_clrinterop_com_callable_wrappers);
    netframework_mark_chart_obsolete(&p->st_clrinterop_marshalling);
    netframework_mark_chart_obsolete(&p->st_clrinterop_interop_stubs_created);
    netframework_mark_chart_obsolete(&p->st_clrjit_methods);
    netframework_mark_chart_obsolete(&p->st_clrjit_time);
    netframework_mark_chart_obsolete(&p->st_clrjit_standard_failures);
    netframework_mark_chart_obsolete(&p->st_clrjit_il_bytes);
    netframework_mark_chart_obsolete(&p->st_clrloading_heap_size);
    netframework_mark_chart_obsolete(&p->st_clrloading_current_appdomains);
    netframework_mark_chart_obsolete(&p->st_clrloading_current_assemblies);
    netframework_mark_chart_obsolete(&p->st_clrloading_current_classes_loaded);
    netframework_mark_chart_obsolete(&p->st_clrloading_app_domains_loaded);
    netframework_mark_chart_obsolete(&p->st_clrloading_app_domains_unloaded);
    netframework_mark_chart_obsolete(&p->st_clrloading_assemblies_loaded);
    netframework_mark_chart_obsolete(&p->st_clrloading_classes_loaded);
    netframework_mark_chart_obsolete(&p->st_clrloading_class_load_failure);
    netframework_mark_chart_obsolete(&p->st_clrmemory_allocated_bytes);
    netframework_mark_chart_obsolete(&p->st_clrmemory_finalization_survivors);
    netframework_mark_chart_obsolete(&p->st_clrmemory_heap_size);
    netframework_mark_chart_obsolete(&p->st_clrmemory_promoted_bytes);
    netframework_mark_chart_obsolete(&p->st_clrmemory_gc_handles);
    netframework_mark_chart_obsolete(&p->st_clrmemory_collections);
    netframework_mark_chart_obsolete(&p->st_clrmemory_induced_gc);
    netframework_mark_chart_obsolete(&p->st_clrmemory_pinned_objects);
    netframework_mark_chart_obsolete(&p->st_clrmemory_sink_blocks_in_use);
    netframework_mark_chart_obsolete(&p->st_clrmemory_committed_bytes);
    netframework_mark_chart_obsolete(&p->st_clrmemory_reserved_bytes);
    netframework_mark_chart_obsolete(&p->st_clrmemory_gc_time);
    netframework_mark_chart_obsolete(&p->st_clrremoting_channels);
    netframework_mark_chart_obsolete(&p->st_clrremoting_context_bound_classes_loaded);
    netframework_mark_chart_obsolete(&p->st_clrremoting_context_bound_objects);
    netframework_mark_chart_obsolete(&p->st_clrremoting_context_proxies);
    netframework_mark_chart_obsolete(&p->st_clrremoting_contexts);
    netframework_mark_chart_obsolete(&p->st_clrremoting_remote_calls);
    netframework_mark_chart_obsolete(&p->st_clrsecurity_link_time_checks);
    netframework_mark_chart_obsolete(&p->st_clrsecurity_rt_checks_time);
    netframework_mark_chart_obsolete(&p->st_clrsecurity_stack_walk_depth);
    netframework_mark_chart_obsolete(&p->st_clrsecurity_run_time_checks);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_queue_length);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_current_queue_length);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_current_logical_threads);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_current_physical_threads);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_recognized_threads);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_current_recognized_threads);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_queue_length_peak);
    netframework_mark_chart_obsolete(&p->st_clrlocksandthreads_contentions);
}

static void dict_net_framework_processes_delete_cb(
    const DICTIONARY_ITEM *item __maybe_unused,
    void *value,
    void *data __maybe_unused)
{
    struct net_framework_instances *p = value;
    netframework_process_cleanup(p);
}

static inline struct net_framework_instances *netframework_process_get(const char *name)
{
    struct net_framework_instances *p = dictionary_set(processes, name, NULL, sizeof(*p));
    p->last_collected = netframework_now_ut;
    return p;
}

static void initialize(void)
{
    processes = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct net_framework_instances));

    dictionary_register_insert_callback(processes, dict_net_framework_processes_insert_cb, NULL);
    dictionary_register_delete_callback(processes, dict_net_framework_processes_delete_cb, NULL);
}

static void
netdata_framework_clr_exceptions(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRExceptionThrown)) {
            if (!p->st_clrexception_thrown) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_thrown", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrexception_thrown = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "exceptions",
                    "netframework.clrexception_thrown",
                    "Thrown exceptions",
                    "exceptions/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_EXCEPTION_THROWN,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrexception_thrown =
                    rrddim_add(p->st_clrexception_thrown, "exceptions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrexception_thrown->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrexception_thrown,
                p->rd_clrexception_thrown,
                (collected_number)p->NETFrameworkCLRExceptionThrown.current.Data);
            rrdset_done(p->st_clrexception_thrown);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRExceptionFilters)) {
            if (!p->st_clrexception_filters) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_filters", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrexception_filters = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "exceptions",
                    "netframework.clrexception_filters",
                    "Thrown exceptions filters",
                    "filters/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_EXCEPTION_FILTERS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrexception_filters =
                    rrddim_add(p->st_clrexception_filters, "filters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrexception_filters->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrexception_filters,
                p->rd_clrexception_filters,
                (collected_number)p->NETFrameworkCLRExceptionFilters.current.Data);
            rrdset_done(p->st_clrexception_filters);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRExceptionFinallys)) {
            if (!p->st_clrexception_finallys) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_finallys", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrexception_finallys = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "exceptions",
                    "netframework.clrexception_finallys",
                    "Executed finally blocks",
                    "finallys/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_EXCEPTION_FINALLYS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrexception_finallys =
                    rrddim_add(p->st_clrexception_finallys, "finallys", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrexception_finallys->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrexception_finallys,
                p->rd_clrexception_finallys,
                (collected_number)p->NETFrameworkCLRExceptionFinallys.current.Data);
            rrdset_done(p->st_clrexception_finallys);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRExceptionTotalCatchDepth)) {
            if (!p->st_clrexception_total_catch_depth) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_throw_to_catch_depth", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrexception_total_catch_depth = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "exceptions",
                    "netframework.clrexception_throw_to_catch_depth",
                    "Traversed stack frames",
                    "stack_frames/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_EXCEPTION_THROW_TO_CATCH_DEPTH,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrexception_total_catch_depth = rrddim_add(
                    p->st_clrexception_total_catch_depth, "traversed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrexception_total_catch_depth->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrexception_total_catch_depth,
                p->rd_clrexception_total_catch_depth,
                (collected_number)p->NETFrameworkCLRExceptionTotalCatchDepth.current.Data);
            rrdset_done(p->st_clrexception_total_catch_depth);
        }
    }
}

static void netdata_framework_clr_interop(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRInteropCOMCallableWrappers)) {
            if (!p->st_clrinterop_com_callable_wrappers) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_com_callable_wrappers", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrinterop_com_callable_wrappers = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "interop",
                    "netframework.clrinterop_com_callable_wrappers",
                    "COM callable wrappers (CCW)",
                    "ccw/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_INTEROP_CCW,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrinterop_com_callable_wrappers = rrddim_add(
                    p->st_clrinterop_com_callable_wrappers,
                    "com_callable_wrappers",
                    NULL,
                    1,
                    1,
                    RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrinterop_com_callable_wrappers->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrinterop_com_callable_wrappers,
                p->rd_clrinterop_com_callable_wrappers,
                (collected_number)p->NETFrameworkCLRInteropCOMCallableWrappers.current.Data);
            rrdset_done(p->st_clrinterop_com_callable_wrappers);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRInteropMarshalling)) {
            if (!p->st_clrinterop_marshalling) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_interop_marshalling", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrinterop_marshalling = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "interop",
                    "netframework.clrinterop_interop_marshallings",
                    "Arguments and return values marshallings",
                    "marshalling/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_INTEROP_MARSHALLING,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrinterop_marshalling =
                    rrddim_add(p->st_clrinterop_marshalling, "marshallings", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrinterop_marshalling->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrinterop_marshalling,
                p->rd_clrinterop_marshalling,
                (collected_number)p->NETFrameworkCLRInteropMarshalling.current.Data);
            rrdset_done(p->st_clrinterop_marshalling);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRInteropStubsCreated)) {
            if (!p->st_clrinterop_interop_stubs_created) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_interop_stubs_created", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrinterop_interop_stubs_created = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "interop",
                    "netframework.clrinterop_interop_stubs_created",
                    "Created stubs",
                    "stubs/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_INTEROP_STUBS_CREATED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrinterop_interop_stubs_created = rrddim_add(
                    p->st_clrinterop_interop_stubs_created, "created", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrinterop_interop_stubs_created->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrinterop_interop_stubs_created,
                p->rd_clrinterop_interop_stubs_created,
                (collected_number)p->NETFrameworkCLRInteropStubsCreated.current.Data);
            rrdset_done(p->st_clrinterop_interop_stubs_created);
        }
    }
}

static void netdata_framework_clr_jit(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRJITMethods)) {
            if (!p->st_clrjit_methods) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_methods", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrjit_methods = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "jit",
                    "netframework.clrjit_methods",
                    "JIT-compiled methods",
                    "methods/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_JIT_METHODS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrjit_methods =
                    rrddim_add(p->st_clrjit_methods, "jit-compiled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrjit_methods->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrjit_methods,
                p->rd_clrjit_methods,
                (collected_number)p->NETFrameworkCLRJITMethods.current.Data);
            rrdset_done(p->st_clrjit_methods);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRJITFrequencyTime) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRJITPercentTime)) {
            if (!p->st_clrjit_time) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_time", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrjit_time = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "jit",
                    "netframework.clrjit_time",
                    "Time spent in JIT compilation",
                    "percentage",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_JIT_TIME,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrjit_time = rrddim_add(p->st_clrjit_time, "time", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrjit_time->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            double percTime = (double)p->NETFrameworkCLRJITPercentTime.current.Data;
            percTime /= (double)p->NETFrameworkCLRJITFrequencyTime.current.Data;
            percTime *= 100;
            rrddim_set_by_pointer(p->st_clrjit_time, p->rd_clrjit_time, (collected_number)percTime);
            rrdset_done(p->st_clrjit_time);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRJITStandardFailures)) {
            if (!p->st_clrjit_standard_failures) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_standard_failures", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrjit_standard_failures = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "jit",
                    "netframework.clrjit_standard_failures",
                    "JIT compiler failures",
                    "failures/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_JIT_STANDARD_FAILURES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrjit_standard_failures =
                    rrddim_add(p->st_clrjit_standard_failures, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrjit_standard_failures->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrjit_standard_failures,
                p->rd_clrjit_standard_failures,
                (collected_number)p->NETFrameworkCLRJITStandardFailures.current.Data);
            rrdset_done(p->st_clrjit_standard_failures);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRJITIlBytes)) {
            if (!p->st_clrjit_il_bytes) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_il_bytes", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrjit_il_bytes = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "jit",
                    "netframework.clrjit_il_bytes",
                    "Compiled Microsoft intermediate language (MSIL) bytes",
                    "bytes/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_JIT_IL_BYTES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrjit_il_bytes =
                    rrddim_add(p->st_clrjit_il_bytes, "compiled_msil", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrjit_il_bytes->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrjit_il_bytes,
                p->rd_clrjit_il_bytes,
                (collected_number)p->NETFrameworkCLRJITIlBytes.current.Data);
            rrdset_done(p->st_clrjit_il_bytes);
        }
    }
}

static void netdata_framework_clr_loading(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingHeapSize)) {
            if (!p->st_clrloading_heap_size) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_loader_heap_size", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_heap_size = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_loader_heap_size",
                    "Memory committed by class loader",
                    "bytes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_HEAP_SIZE,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_heap_size =
                    rrddim_add(p->st_clrloading_heap_size, "committed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrloading_heap_size->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_heap_size,
                p->rd_clrloading_heap_size,
                (collected_number)p->NETFrameworkCLRLoadingHeapSize.current.Data);
            rrdset_done(p->st_clrloading_heap_size);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingCurrentAppdomains)) {
            if (!p->st_clrloading_current_appdomains) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_current_appdomains", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_current_appdomains = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_current_appdomains",
                    "Current application domains",
                    "domains",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_CURRENT_APP_DOMAINS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_current_appdomains =
                    rrddim_add(p->st_clrloading_current_appdomains, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrloading_current_appdomains->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_current_appdomains,
                p->rd_clrloading_current_appdomains,
                (collected_number)p->NETFrameworkCLRLoadingCurrentAppdomains.current.Data);
            rrdset_done(p->st_clrloading_current_appdomains);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingCurrentAssemblies)) {
            if (!p->st_clrloading_current_assemblies) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_current_assemblies", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_current_assemblies = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_current_assemblies",
                    "Current loaded assemblies",
                    "assemblies",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_CURRENT_ASSEMBLIES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_current_assemblies =
                    rrddim_add(p->st_clrloading_current_assemblies, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrloading_current_assemblies->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_current_assemblies,
                p->rd_clrloading_current_assemblies,
                (collected_number)p->NETFrameworkCLRLoadingCurrentAssemblies.current.Data);
            rrdset_done(p->st_clrloading_current_assemblies);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingCurrentClassesLoaded)) {
            if (!p->st_clrloading_current_classes_loaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_current_classes_loaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_current_classes_loaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_current_classes_loaded",
                    "Current loaded classes",
                    "classes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_CURRENT_CLASSES_LOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_current_classes_loaded = rrddim_add(
                    p->st_clrloading_current_classes_loaded, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrloading_current_classes_loaded->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_current_classes_loaded,
                p->rd_clrloading_current_classes_loaded,
                (collected_number)p->NETFrameworkCLRLoadingCurrentClassesLoaded.current.Data);
            rrdset_done(p->st_clrloading_current_classes_loaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingAppDomainsLoaded)) {
            if (!p->st_clrloading_app_domains_loaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_appdomains_loaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_app_domains_loaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_appdomains_loaded",
                    "Loaded application domains",
                    "domain/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_APP_DOMAINS_LOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_app_domains_loaded =
                    rrddim_add(p->st_clrloading_app_domains_loaded, "loaded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrloading_app_domains_loaded->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_app_domains_loaded,
                p->rd_clrloading_app_domains_loaded,
                (collected_number)p->NETFrameworkCLRLoadingAppDomainsLoaded.current.Data);
            rrdset_done(p->st_clrloading_app_domains_loaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingAppDomainsUnloaded)) {
            if (!p->st_clrloading_app_domains_unloaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_appdomains_unloaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_app_domains_unloaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_appdomains_unloaded",
                    "Unloaded application domains",
                    "domain/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_APP_DOMAINS_UNLOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_app_domains_unloaded = rrddim_add(
                    p->st_clrloading_app_domains_unloaded, "unloaded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrloading_app_domains_unloaded->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_app_domains_unloaded,
                p->rd_clrloading_app_domains_unloaded,
                (collected_number)p->NETFrameworkCLRLoadingAppDomainsUnloaded.current.Data);
            rrdset_done(p->st_clrloading_app_domains_unloaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingAssembliesLoaded)) {
            if (!p->st_clrloading_assemblies_loaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_assemblies_loaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_assemblies_loaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_assemblies_loaded",
                    "Loaded assemblies",
                    "assemblies/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_ASSEMBLIES_LOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_assemblies_loaded =
                    rrddim_add(p->st_clrloading_assemblies_loaded, "loaded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrloading_assemblies_loaded->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_assemblies_loaded,
                p->rd_clrloading_assemblies_loaded,
                (collected_number)p->NETFrameworkCLRLoadingAssembliesLoaded.current.Data);
            rrdset_done(p->st_clrloading_assemblies_loaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingClassesLoaded)) {
            if (!p->st_clrloading_classes_loaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_classes_loaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_classes_loaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_classes_loaded",
                    "Loaded classes in all assemblies",
                    "classes/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_CLASSES_LOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_classes_loaded =
                    rrddim_add(p->st_clrloading_classes_loaded, "loaded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrloading_classes_loaded->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_classes_loaded,
                p->rd_clrloading_classes_loaded,
                (collected_number)p->NETFrameworkCLRLoadingClassesLoaded.current.Data);
            rrdset_done(p->st_clrloading_classes_loaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLoadingClassLoadFailure)) {
            if (!p->st_clrloading_class_load_failure) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_class_load_failure", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrloading_class_load_failure = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "loading",
                    "netframework.clrloading_class_load_failures",
                    "Class load failures",
                    "failures/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOADING_CLASS_LOAD_FAILURE,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrloading_class_load_failure = rrddim_add(
                    p->st_clrloading_class_load_failure, "class_load", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrloading_class_load_failure->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrloading_class_load_failure,
                p->rd_clrloading_class_load_failure,
                (collected_number)p->NETFrameworkCLRLoadingClassLoadFailure.current.Data);
            rrdset_done(p->st_clrloading_class_load_failure);
        }
    }
}

static void netdata_framework_clr_memory(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;

    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        bool has_allocated_bytes =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryAllocatedBytesPerSec);
        bool has_finalization_survivors =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryFinalizationSurvivors);
        bool has_gen0_heap =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen0HeapSize);
        bool has_gen0_promoted =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen0PromotedBytesPerSec);
        bool has_gen1_heap =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen1HeapSize);
        bool has_gen1_promoted =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen1PromotedBytesPerSec);
        bool has_gen2_heap =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen2HeapSize);
        bool has_loh_heap =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryLargeObjectHeapSize);
        bool has_gc_handles =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGCHandles);
        bool has_gen0_collections =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen0Collections);
        bool has_gen1_collections =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen1Collections);
        bool has_gen2_collections =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryGen2Collections);
        bool has_induced_gc =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryInducedGC);
        bool has_pinned_objects =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryPinnedObjects);
        bool has_sink_blocks =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemorySinkBlocksInUse);
        bool has_committed_bytes =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryCommittedBytes);
        bool has_reserved_bytes =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryReservedBytes);
        bool has_gc_time =
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRMemoryTimeInGC);

        if (has_allocated_bytes) {
            if (!p->st_clrmemory_allocated_bytes) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_allocated_bytes", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_allocated_bytes = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_allocated_bytes",
                    "Allocated bytes on the GC heap",
                    "bytes/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_ALLOCATED_BYTES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_allocated_bytes =
                    rrddim_add(p->st_clrmemory_allocated_bytes, "allocated", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrmemory_allocated_bytes->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_allocated_bytes,
                p->rd_clrmemory_allocated_bytes,
                (collected_number)p->NETFrameworkCLRMemoryAllocatedBytesPerSec.current.Data);
            rrdset_done(p->st_clrmemory_allocated_bytes);
        }

        if (has_finalization_survivors) {
            if (!p->st_clrmemory_finalization_survivors) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_finalization_survivors", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_finalization_survivors = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_finalization_survivors",
                    "Finalization survivors",
                    "objects",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_FINALIZATION_SURVIVORS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_finalization_survivors = rrddim_add(
                    p->st_clrmemory_finalization_survivors, "survivors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrmemory_finalization_survivors->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_finalization_survivors,
                p->rd_clrmemory_finalization_survivors,
                (collected_number)p->NETFrameworkCLRMemoryFinalizationSurvivors.current.Data);
            rrdset_done(p->st_clrmemory_finalization_survivors);
        }

        if (has_gen0_heap || has_gen1_heap || has_gen2_heap || has_loh_heap) {
            if (!p->st_clrmemory_heap_size) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_heap_size", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_heap_size = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_heap_size",
                    "Heap size by generation",
                    "bytes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_HEAP_SIZE,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_heap_gen0 =
                    rrddim_add(p->st_clrmemory_heap_size, "gen0", "gen0", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_clrmemory_heap_gen1 =
                    rrddim_add(p->st_clrmemory_heap_size, "gen1", "gen1", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_clrmemory_heap_gen2 =
                    rrddim_add(p->st_clrmemory_heap_size, "gen2", "gen2", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p->rd_clrmemory_heap_loh =
                    rrddim_add(p->st_clrmemory_heap_size, "loh", "loh", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrmemory_heap_size->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            if (has_gen0_heap)
                rrddim_set_by_pointer(
                    p->st_clrmemory_heap_size,
                    p->rd_clrmemory_heap_gen0,
                    (collected_number)p->NETFrameworkCLRMemoryGen0HeapSize.current.Data);
            if (has_gen1_heap)
                rrddim_set_by_pointer(
                    p->st_clrmemory_heap_size,
                    p->rd_clrmemory_heap_gen1,
                    (collected_number)p->NETFrameworkCLRMemoryGen1HeapSize.current.Data);
            if (has_gen2_heap)
                rrddim_set_by_pointer(
                    p->st_clrmemory_heap_size,
                    p->rd_clrmemory_heap_gen2,
                    (collected_number)p->NETFrameworkCLRMemoryGen2HeapSize.current.Data);
            if (has_loh_heap)
                rrddim_set_by_pointer(
                    p->st_clrmemory_heap_size,
                    p->rd_clrmemory_heap_loh,
                    (collected_number)p->NETFrameworkCLRMemoryLargeObjectHeapSize.current.Data);
            rrdset_done(p->st_clrmemory_heap_size);
        }

        if (has_gen0_promoted || has_gen1_promoted) {
            if (!p->st_clrmemory_promoted_bytes) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_promoted_bytes", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_promoted_bytes = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_promoted_bytes",
                    "Promoted bytes by generation",
                    "bytes/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_PROMOTED_BYTES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_promoted_gen0 =
                    rrddim_add(p->st_clrmemory_promoted_bytes, "gen0", "gen0", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_clrmemory_promoted_gen1 =
                    rrddim_add(p->st_clrmemory_promoted_bytes, "gen1", "gen1", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrmemory_promoted_bytes->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            if (has_gen0_promoted)
                rrddim_set_by_pointer(
                    p->st_clrmemory_promoted_bytes,
                    p->rd_clrmemory_promoted_gen0,
                    (collected_number)p->NETFrameworkCLRMemoryGen0PromotedBytesPerSec.current.Data);
            if (has_gen1_promoted)
                rrddim_set_by_pointer(
                    p->st_clrmemory_promoted_bytes,
                    p->rd_clrmemory_promoted_gen1,
                    (collected_number)p->NETFrameworkCLRMemoryGen1PromotedBytesPerSec.current.Data);
            rrdset_done(p->st_clrmemory_promoted_bytes);
        }

        if (has_gc_handles) {
            if (!p->st_clrmemory_gc_handles) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_gc_handles", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_gc_handles = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_gc_handles",
                    "GC handles in use",
                    "handles",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_GC_HANDLES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_gc_handles =
                    rrddim_add(p->st_clrmemory_gc_handles, "handles", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrmemory_gc_handles->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_gc_handles,
                p->rd_clrmemory_gc_handles,
                (collected_number)p->NETFrameworkCLRMemoryGCHandles.current.Data);
            rrdset_done(p->st_clrmemory_gc_handles);
        }

        if (has_gen0_collections || has_gen1_collections || has_gen2_collections) {
            if (!p->st_clrmemory_collections) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_collections", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_collections = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_collections",
                    "Garbage collections by generation",
                    "collections/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_COLLECTIONS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_collections_gen0 =
                    rrddim_add(p->st_clrmemory_collections, "gen0", "gen0", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_clrmemory_collections_gen1 =
                    rrddim_add(p->st_clrmemory_collections, "gen1", "gen1", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_clrmemory_collections_gen2 =
                    rrddim_add(p->st_clrmemory_collections, "gen2", "gen2", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrmemory_collections->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            if (has_gen0_collections)
                rrddim_set_by_pointer(
                    p->st_clrmemory_collections,
                    p->rd_clrmemory_collections_gen0,
                    (collected_number)p->NETFrameworkCLRMemoryGen0Collections.current.Data);
            if (has_gen1_collections)
                rrddim_set_by_pointer(
                    p->st_clrmemory_collections,
                    p->rd_clrmemory_collections_gen1,
                    (collected_number)p->NETFrameworkCLRMemoryGen1Collections.current.Data);
            if (has_gen2_collections)
                rrddim_set_by_pointer(
                    p->st_clrmemory_collections,
                    p->rd_clrmemory_collections_gen2,
                    (collected_number)p->NETFrameworkCLRMemoryGen2Collections.current.Data);
            rrdset_done(p->st_clrmemory_collections);
        }

        if (has_induced_gc) {
            if (!p->st_clrmemory_induced_gc) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_induced_gc", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_induced_gc = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_induced_gc",
                    "Induced garbage collections",
                    "collections/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_INDUCED_GC,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_induced_gc =
                    rrddim_add(p->st_clrmemory_induced_gc, "induced", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrmemory_induced_gc->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_induced_gc,
                p->rd_clrmemory_induced_gc,
                (collected_number)p->NETFrameworkCLRMemoryInducedGC.current.Data);
            rrdset_done(p->st_clrmemory_induced_gc);
        }

        if (has_pinned_objects) {
            if (!p->st_clrmemory_pinned_objects) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_pinned_objects", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_pinned_objects = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_pinned_objects",
                    "Pinned objects",
                    "objects",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_PINNED_OBJECTS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_pinned_objects =
                    rrddim_add(p->st_clrmemory_pinned_objects, "pinned", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrmemory_pinned_objects->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_pinned_objects,
                p->rd_clrmemory_pinned_objects,
                (collected_number)p->NETFrameworkCLRMemoryPinnedObjects.current.Data);
            rrdset_done(p->st_clrmemory_pinned_objects);
        }

        if (has_sink_blocks) {
            if (!p->st_clrmemory_sink_blocks_in_use) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_sink_blocks_in_use", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_sink_blocks_in_use = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_sink_blocks_in_use",
                    "Sink blocks in use",
                    "blocks",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_SINK_BLOCKS_IN_USE,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_sink_blocks_in_use =
                    rrddim_add(p->st_clrmemory_sink_blocks_in_use, "in_use", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrmemory_sink_blocks_in_use->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_sink_blocks_in_use,
                p->rd_clrmemory_sink_blocks_in_use,
                (collected_number)p->NETFrameworkCLRMemorySinkBlocksInUse.current.Data);
            rrdset_done(p->st_clrmemory_sink_blocks_in_use);
        }

        if (has_committed_bytes) {
            if (!p->st_clrmemory_committed_bytes) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_committed_bytes", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_committed_bytes = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_committed_bytes",
                    "Committed GC virtual memory",
                    "bytes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_COMMITTED_BYTES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_committed_bytes =
                    rrddim_add(p->st_clrmemory_committed_bytes, "committed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrmemory_committed_bytes->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_committed_bytes,
                p->rd_clrmemory_committed_bytes,
                (collected_number)p->NETFrameworkCLRMemoryCommittedBytes.current.Data);
            rrdset_done(p->st_clrmemory_committed_bytes);
        }

        if (has_reserved_bytes) {
            if (!p->st_clrmemory_reserved_bytes) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_reserved_bytes", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_reserved_bytes = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_reserved_bytes",
                    "Reserved GC virtual memory",
                    "bytes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_RESERVED_BYTES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_reserved_bytes =
                    rrddim_add(p->st_clrmemory_reserved_bytes, "reserved", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrmemory_reserved_bytes->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrmemory_reserved_bytes,
                p->rd_clrmemory_reserved_bytes,
                (collected_number)p->NETFrameworkCLRMemoryReservedBytes.current.Data);
            rrdset_done(p->st_clrmemory_reserved_bytes);
        }

        if (has_gc_time) {
            if (!p->st_clrmemory_gc_time) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrmemory_gc_time", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrmemory_gc_time = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "memory",
                    "netframework.clrmemory_gc_time",
                    "Time spent in garbage collection",
                    "percentage",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_MEMORY_GC_TIME,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrmemory_gc_time =
                    rrddim_add(p->st_clrmemory_gc_time, "time", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrmemory_gc_time->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            NETDATA_DOUBLE value = 0;
            if (p->NETFrameworkCLRMemoryTimeInGC.current.Time)
                value = 100.0 * (NETDATA_DOUBLE)p->NETFrameworkCLRMemoryTimeInGC.current.Data /
                        (NETDATA_DOUBLE)p->NETFrameworkCLRMemoryTimeInGC.current.Time;

            rrddim_set_by_pointer(p->st_clrmemory_gc_time, p->rd_clrmemory_gc_time, (collected_number)value);
            rrdset_done(p->st_clrmemory_gc_time);
        }
    }
}

static void netdata_framework_clr_remoting(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingChannels)) {
            if (!p->st_clrremoting_channels) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_channels", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_channels = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_channels",
                    "Registered channels",
                    "channels/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_CHANNELS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_channels =
                    rrddim_add(p->st_clrremoting_channels, "registered", "registered", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrremoting_channels->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_channels,
                p->rd_clrremoting_channels,
                (collected_number)p->NETFrameworkCLRRemotingChannels.current.Data);
            rrdset_done(p->st_clrremoting_channels);
        }

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingContextBoundClassesLoaded)) {
            if (!p->st_clrremoting_context_bound_classes_loaded) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_context_bound_classes_loaded", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_context_bound_classes_loaded = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_context_bound_classes_loaded",
                    "Loaded context-bound classes",
                    "classes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_CONTEXT_BOUND_CLASSES_LOADED,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_context_bound_classes_loaded = rrddim_add(
                    p->st_clrremoting_context_bound_classes_loaded, "loaded", "loaded", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrremoting_context_bound_classes_loaded->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_context_bound_classes_loaded,
                p->rd_clrremoting_context_bound_classes_loaded,
                (collected_number)p->NETFrameworkCLRRemotingContextBoundClassesLoaded.current.Data);
            rrdset_done(p->st_clrremoting_context_bound_classes_loaded);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingContextBoundObjects)) {
            if (!p->st_clrremoting_context_bound_objects) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_context_bound_objects", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_context_bound_objects = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_context_bound_objects",
                    "Allocated context-bound objects",
                    "objects/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_CONTEXT_BOUND_OBJECTS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_context_bound_objects = rrddim_add(
                    p->st_clrremoting_context_bound_objects, "allocated", "allocated", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrremoting_context_bound_objects->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_context_bound_objects,
                p->rd_clrremoting_context_bound_objects,
                (collected_number)p->NETFrameworkCLRRemotingContextBoundObjects.current.Data);
            rrdset_done(p->st_clrremoting_context_bound_objects);
        }
        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingContextProxies)) {
            if (!p->st_clrremoting_context_proxies) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_context_proxies", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_context_proxies = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_context_proxies",
                    "Remoting proxy objects",
                    "objects/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_CONTEXTS_PROXIES,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_context_proxies =
                    rrddim_add(p->st_clrremoting_context_proxies, "objects", "objects", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrremoting_context_proxies->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_context_proxies,
                p->rd_clrremoting_context_proxies,
                (collected_number)p->NETFrameworkCLRRemotingContextProxies.current.Data);
            rrdset_done(p->st_clrremoting_context_proxies);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingContexts)) {
            if (!p->st_clrremoting_contexts) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_contexts", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_contexts = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_contexts",
                    "Total of remoting contexts",
                    "contexts",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_CONTEXTS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_contexts =
                    rrddim_add(p->st_clrremoting_contexts, "contexts", "contexts", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrremoting_contexts->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_contexts,
                p->rd_clrremoting_contexts,
                (collected_number)p->NETFrameworkCLRRemotingContexts.current.Data);
            rrdset_done(p->st_clrremoting_contexts);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRRemotingRemoteCalls)) {
            if (!p->st_clrremoting_remote_calls) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrremoting_calls", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrremoting_remote_calls = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "remoting",
                    "netframework.clrremoting_remote_calls",
                    "Remote Procedure Calls (RPC) invoked",
                    "calls/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_REMOTING_REMOTE_CALLS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrremoting_remote_calls =
                    rrddim_add(p->st_clrremoting_remote_calls, "calls", "calls", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrremoting_remote_calls->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrremoting_remote_calls,
                p->rd_clrremoting_remote_calls,
                (collected_number)p->NETFrameworkCLRRemotingRemoteCalls.current.Data);
            rrdset_done(p->st_clrremoting_remote_calls);
        }
    }
}

static void netdata_framework_clr_security(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRSecurityLinkTimeChecks)) {
            if (!p->st_clrsecurity_link_time_checks) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrsecurity_link_time_checks", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrsecurity_link_time_checks = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "security",
                    "netframework.clrsecurity_link_time_checks",
                    "Link-time code access security checks",
                    "checks/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_SECURITY_LINK_TIME_CHECKS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrsecurity_link_time_checks =
                    rrddim_add(p->st_clrsecurity_link_time_checks, "linktime", "linktime", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrsecurity_link_time_checks->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrsecurity_link_time_checks,
                p->rd_clrsecurity_link_time_checks,
                (collected_number)p->NETFrameworkCLRSecurityLinkTimeChecks.current.Data);
            rrdset_done(p->st_clrsecurity_link_time_checks);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRSecurityPercentTimeinRTChecks) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRSecurityFrequency_PerfTime)) {
            if (!p->st_clrsecurity_rt_checks_time) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrsecurity_checks_time", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrsecurity_rt_checks_time = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "security",
                    "netframework.clrsecurity_checks_time",
                    "Time spent performing runtime code access security checks.",
                    "percentage",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_SECURITY_RT_CHECKS_TIME,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrsecurity_rt_checks_time =
                    rrddim_add(p->st_clrsecurity_rt_checks_time, "time", "time", 1, 100, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrsecurity_rt_checks_time->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            NETDATA_DOUBLE value = (NETDATA_DOUBLE)p->NETFrameworkCLRSecurityPercentTimeinRTChecks.current.Data;
            value /= (NETDATA_DOUBLE)p->NETFrameworkCLRSecurityFrequency_PerfTime.current.Data;

            rrddim_set_by_pointer(
                p->st_clrsecurity_rt_checks_time,
                p->rd_clrsecurity_rt_checks_time,
                (collected_number)(value * 100.0));
            rrdset_done(p->st_clrsecurity_rt_checks_time);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRSecurityStackWalkDepth)) {
            if (!p->st_clrsecurity_stack_walk_depth) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrsecurity_stack_walk_depth", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrsecurity_stack_walk_depth = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "security",
                    "netframework.clrsecurity_stack_walk_depth",
                    "Depth of the stack",
                    "depth",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_SECURITY_STACK_WALK_DEPTH,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrsecurity_stack_walk_depth =
                    rrddim_add(p->st_clrsecurity_stack_walk_depth, "stack", "stack", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrsecurity_stack_walk_depth->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrsecurity_stack_walk_depth,
                p->rd_clrsecurity_stack_walk_depth,
                (collected_number)p->NETFrameworkCLRSecurityStackWalkDepth.current.Data);
            rrdset_done(p->st_clrsecurity_stack_walk_depth);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRSecurityRunTimeChecks)) {
            if (!p->st_clrsecurity_run_time_checks) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrsecurity_runtime_checks", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrsecurity_run_time_checks = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "security",
                    "netframework.clrsecurity_runtime_checks",
                    "Runtime code access security checks performed",
                    "checks/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_SECURITY_RUNTIME_CHECKS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_clrsecurity_run_time_checks =
                    rrddim_add(p->st_clrsecurity_run_time_checks, "runtime", "runtime", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrsecurity_run_time_checks->rrdlabels, "process", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrsecurity_run_time_checks,
                p->rd_clrsecurity_run_time_checks,
                (collected_number)p->NETFrameworkCLRSecurityRunTimeChecks.current.Data);
            rrdset_done(p->st_clrsecurity_run_time_checks);
        }
    }
}

static void
netdata_framework_clr_locks_and_threads(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        struct net_framework_instances *p = netframework_process_get(windows_shared_buffer);

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsCurrentQueueLength)) {
            if (!p->st_clrlocksandthreads_current_queue_length) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_current_queue_length", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_current_queue_length = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_current_queue_length",
                    "Current managed lock waiters",
                    "threads",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_CURRENT_QUEUE_LENGTH,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_current_queue_length =
                    rrddim_add(p->st_clrlocksandthreads_current_queue_length, "threads", "threads", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrlocksandthreads_current_queue_length->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_current_queue_length,
                p->rd_locksandthreads_current_queue_length,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsCurrentQueueLength.current.Data);
            rrdset_done(p->st_clrlocksandthreads_current_queue_length);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsQueueLength)) {
            if (!p->st_clrlocksandthreads_queue_length) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_queue_length", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_queue_length = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_queue_length",
                    "Threads waited to acquire a managed lock",
                    "threads/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_QUEUE_LENGTH,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_queue_length =
                    rrddim_add(p->st_clrlocksandthreads_queue_length, "threads", "threads", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrlocksandthreads_queue_length->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_queue_length,
                p->rd_locksandthreads_queue_length,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsQueueLength.current.Data);
            rrdset_done(p->st_clrlocksandthreads_queue_length);
        }

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsCurrentLogicalThreads)) {
            if (!p->st_clrlocksandthreads_current_logical_threads) {
                snprintfz(
                    id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_current_logical_threads", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_current_logical_threads = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_current_logical_threads",
                    "Logical threads",
                    "threads",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_LOGICAL_THREADS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_current_logical_threads = rrddim_add(
                    p->st_clrlocksandthreads_current_logical_threads, "logical", "logical", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrlocksandthreads_current_logical_threads->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_current_logical_threads,
                p->rd_locksandthreads_current_logical_threads,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsCurrentLogicalThreads.current.Data);
            rrdset_done(p->st_clrlocksandthreads_current_logical_threads);
        }

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads)) {
            if (!p->st_clrlocksandthreads_current_physical_threads) {
                snprintfz(
                    id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_current_physical_threads", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_current_physical_threads = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_current_physical_threads",
                    "Physical threads",
                    "threads",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_CURRENT_PHYSICAL_THREADS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_current_physical_threads = rrddim_add(
                    p->st_clrlocksandthreads_current_physical_threads, "physical", "physical", 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrlocksandthreads_current_physical_threads->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_current_physical_threads,
                p->rd_locksandthreads_current_physical_threads,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads.current.Data);
            rrdset_done(p->st_clrlocksandthreads_current_physical_threads);
        }

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsCurrentRecognizedThreads)) {
            if (!p->st_clrlocksandthreads_current_recognized_threads) {
                snprintfz(
                    id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_current_recognized_threads", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_current_recognized_threads = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_current_recognized_threads",
                    "Current recognized threads",
                    "threads",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_CURRENT_RECOGNIZED_THREADS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_current_recognized_threads = rrddim_add(
                    p->st_clrlocksandthreads_current_recognized_threads,
                    "threads",
                    "threads",
                    1,
                    1,
                    RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(
                    p->st_clrlocksandthreads_current_recognized_threads->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_current_recognized_threads,
                p->rd_locksandthreads_current_recognized_threads,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsCurrentRecognizedThreads.current.Data);
            rrdset_done(p->st_clrlocksandthreads_current_recognized_threads);
        }

        if (perflibGetInstanceCounter(
                pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsRecognizedThreads)) {
            if (!p->st_clrlocksandthreads_recognized_threads) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_recognized_threads", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_recognized_threads = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_recognized_threads",
                    "Threads recognized by the runtime",
                    "threads/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_RECOGNIZED_THREADS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_recognized_threads = rrddim_add(
                    p->st_clrlocksandthreads_recognized_threads, "threads", "threads", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrlocksandthreads_recognized_threads->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_recognized_threads,
                p->rd_locksandthreads_recognized_threads,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsRecognizedThreads.current.Data);
            rrdset_done(p->st_clrlocksandthreads_recognized_threads);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsQueueLengthPeak)) {
            if (!p->st_clrlocksandthreads_queue_length_peak) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_queue_length_peak", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_queue_length_peak = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_queue_length_peak",
                    "Threads waited to acquire a managed lock",
                    "threads/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_QUEUE_LENGTH_PEAK,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_queue_length_peak = rrddim_add(
                    p->st_clrlocksandthreads_queue_length_peak, "threads", "threads", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrlocksandthreads_queue_length_peak->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_queue_length_peak,
                p->rd_locksandthreads_queue_length_peak,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsQueueLengthPeak.current.Data);
            rrdset_done(p->st_clrlocksandthreads_queue_length_peak);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->NETFrameworkCLRLocksAndThreadsContentions)) {
            if (!p->st_clrlocksandthreads_contentions) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrlocksandthreads_contentions", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_clrlocksandthreads_contentions = rrdset_create_localhost(
                    "netframework",
                    id,
                    NULL,
                    "locks threads",
                    "netframework.clrlocksandthreads_contentions",
                    "Fails to acquire a managed lock",
                    "contentions/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetFramework",
                    PRIO_NETFRAMEWORK_CLR_LOCKS_AND_THREADS_CONTENTIONS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_locksandthreads_contentions = rrddim_add(
                    p->st_clrlocksandthreads_contentions, "contentions", "contentions", 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_clrlocksandthreads_contentions->rrdlabels,
                    "process",
                    windows_shared_buffer,
                    RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_clrlocksandthreads_contentions,
                p->rd_locksandthreads_contentions,
                (collected_number)p->NETFrameworkCLRLocksAndThreadsContentions.current.Data);
            rrdset_done(p->st_clrlocksandthreads_contentions);
        }
    }
}

struct netdata_netframework_objects {
    char *object;
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
} netframewrk_obj[NETDATA_NETFRAMEWORK_END] = {
    {.fnct = netdata_framework_clr_exceptions, .object = ".NET CLR Exceptions"},
    {.fnct = netdata_framework_clr_interop, .object = ".NET CLR Interop"},
    {.fnct = netdata_framework_clr_jit, .object = ".NET CLR Jit"},
    {.fnct = netdata_framework_clr_loading, .object = ".NET CLR Loading"},
    {.fnct = netdata_framework_clr_memory, .object = ".NET CLR Memory"},
    {.fnct = netdata_framework_clr_remoting, .object = ".NET CLR Remoting"},
    {.fnct = netdata_framework_clr_security, .object = ".NET CLR Security"},
    {.fnct = netdata_framework_clr_locks_and_threads, .object = ".NET CLR LocksAndThreads"}

};

int do_PerflibNetFramework(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    netframework_now_ut = now_monotonic_usec();

    int i;
    for (i = 0; i < NETDATA_NETFRAMEWORK_END; i++) {
        DWORD id = RegistryFindIDByName(netframewrk_obj[i].object);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, netframewrk_obj[i].object);
        if (!pObjectType)
            continue;

        netframewrk_obj[i].fnct(pDataBlock, pObjectType, update_every);
    }

    {
        struct net_framework_instances *p;
        dfe_start_write(processes, p)
        {
            if (p->last_collected < netframework_now_ut)
                dictionary_del(processes, p_dfe.name);
        }
        dfe_done(p);
        dictionary_garbage_collect(processes);
    }

    return 0;
}
