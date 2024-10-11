// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

enum netdata_netframework_metrics {
    NETDATA_NETFRAMEWORK_EXCEPTIONS,
    NETDATA_NETFRAMEWORK_INTEROP,
    NETDATA_NETFRAMEWORK_JIT,
    NETDATA_NETFRAMEWORK_LOADING,

    NETDATA_NETFRAMEWORK_END
};

struct net_framework_instances {
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
    COUNTER_DATA NETFrameworkCLRLoadingAppDomainsLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingAppDomainsUnloaded;
    COUNTER_DATA NETFrameworkCLRLoadingAssembliesLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingClassesLoaded;
    COUNTER_DATA NETFrameworkCLRLoadingClassLoadFailure;
};

static inline void initialize_net_framework_processes_keys(struct net_framework_instances *p) {
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
    p->NETFrameworkCLRLoadingAppDomainsLoaded.key = "Rate of appdomains";
    p->NETFrameworkCLRLoadingAppDomainsUnloaded.key = "Total appdomains unloaded";
    p->NETFrameworkCLRLoadingAssembliesLoaded.key = "Total Assemblies";
    p->NETFrameworkCLRLoadingClassesLoaded.key = "Total Classes Loaded";
    p->NETFrameworkCLRLoadingClassLoadFailure.key = "Total # of Load Failures";
}

void dict_net_framework_processes_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct net_framework_instances *p = value;
    initialize_net_framework_processes_keys(p);
}

static DICTIONARY *processes = NULL;

static void initialize(void) {
    processes = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                               DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct net_framework_instances));

    dictionary_register_insert_callback(processes, dict_net_framework_processes_insert_cb, NULL);
}

static void netdata_framework_clr_exceptions(PERF_DATA_BLOCK *pDataBlock,
                                             PERF_OBJECT_TYPE *pObjectType,
                                             int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        netdata_fix_chart_name(windows_shared_buffer);
        struct net_framework_instances *p = dictionary_set(processes, windows_shared_buffer, NULL, sizeof(*p));

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRExceptionThrown)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_thrown", windows_shared_buffer);
            if (!p->st_clrexception_thrown) {
                p->st_clrexception_thrown = rrdset_create_localhost("netframework"
                                                                    , windows_shared_buffer, NULL
                                                                    , "exceptions"
                                                                    , "netframework.clrexception_thrown"
                                                                    , "Thrown exceptions"
                                                                    , "exceptions/s"
                                                                    , PLUGIN_WINDOWS_NAME
                                                                    , "PerflibNetFramework"
                                                                    , PRIO_NETFRAMEWORK_CLR_EXCEPTION_THROWN
                                                                    , update_every
                                                                    , RRDSET_TYPE_LINE
                                                                    );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrexception_thrown_total", windows_shared_buffer);
                p->rd_clrexception_thrown  = rrddim_add(p->st_clrexception_thrown,
                                                       id,
                                                       "exceptions",
                                                       1,
                                                       1,
                                                       RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrexception_thrown->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrexception_thrown,
                                  p->rd_clrexception_thrown,
                                  (collected_number)p->NETFrameworkCLRExceptionThrown.current.Data);
            rrdset_done(p->st_clrexception_thrown);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRExceptionFilters)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_filters", windows_shared_buffer);
            if (!p->st_clrexception_filters) {
                p->st_clrexception_filters = rrdset_create_localhost("netframework"
                                                                    , windows_shared_buffer, NULL
                                                                    , "exceptions"
                                                                    , "netframework.clrexception_filters"
                                                                    , "Thrown exceptions filters"
                                                                    , "filters/s"
                                                                    , PLUGIN_WINDOWS_NAME
                                                                    , "PerflibNetFramework"
                                                                    , PRIO_NETFRAMEWORK_CLR_EXCEPTION_FILTERS
                                                                    , update_every
                                                                    , RRDSET_TYPE_LINE
                                                                    );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrexception_filters_total", windows_shared_buffer);
                p->rd_clrexception_filters  = rrddim_add(p->st_clrexception_filters,
                                                        id,
                                                        "filters",
                                                        1,
                                                        1,
                                                        RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrexception_filters->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrexception_filters,
                                  p->rd_clrexception_filters,
                                  (collected_number)p->NETFrameworkCLRExceptionFilters.current.Data);
            rrdset_done(p->st_clrexception_filters);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRExceptionFinallys)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_finallys", windows_shared_buffer);
            if (!p->st_clrexception_finallys) {
                p->st_clrexception_finallys = rrdset_create_localhost("netframework"
                                                                      , windows_shared_buffer, NULL
                                                                      , "exceptions"
                                                                      , "netframework.clrexception_finallys"
                                                                      , "Executed finally blocks"
                                                                      , "finallys/s"
                                                                      , PLUGIN_WINDOWS_NAME
                                                                      , "PerflibNetFramework"
                                                                      , PRIO_NETFRAMEWORK_CLR_EXCEPTION_FINALLYS
                                                                      , update_every
                                                                      , RRDSET_TYPE_LINE
                                                                      );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrexception_finallys_total", windows_shared_buffer);
                p->rd_clrexception_finallys  = rrddim_add(p->st_clrexception_finallys,
                                                         id,
                                                         "finallys",
                                                         1,
                                                         1,
                                                         RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrexception_finallys->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrexception_finallys,
                                  p->rd_clrexception_finallys,
                                  (collected_number)p->NETFrameworkCLRExceptionFinallys.current.Data);
            rrdset_done(p->st_clrexception_finallys);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRExceptionTotalCatchDepth)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrexception_throw_to_catch_depth", windows_shared_buffer);
            if (!p->st_clrexception_total_catch_depth) {
                p->st_clrexception_total_catch_depth = rrdset_create_localhost("netframework"
                                                                               , windows_shared_buffer, NULL
                                                                               , "exceptions"
                                                                               , "netframework.clrexception_throw_to_catch_depth"
                                                                               , "Traversed stack frames"
                                                                               , "stack_frames/s"
                                                                               , PLUGIN_WINDOWS_NAME
                                                                               , "PerflibNetFramework"
                                                                               , PRIO_NETFRAMEWORK_CLR_EXCEPTION_THROW_TO_CATCH_DEPTH
                                                                               , update_every
                                                                               , RRDSET_TYPE_LINE
                                                                               );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrexception_throw_to_catch_depth_total", windows_shared_buffer);
                p->rd_clrexception_total_catch_depth  = rrddim_add(p->st_clrexception_total_catch_depth,
                                                                  id,
                                                                  "traversed",
                                                                  1,
                                                                  1,
                                                                  RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrexception_total_catch_depth->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrexception_total_catch_depth,
                                  p->rd_clrexception_total_catch_depth,
                                  (collected_number)p->NETFrameworkCLRExceptionTotalCatchDepth.current.Data);
            rrdset_done(p->st_clrexception_total_catch_depth);
        }
    }
}

static void netdata_framework_clr_interop(PERF_DATA_BLOCK *pDataBlock,
                                          PERF_OBJECT_TYPE *pObjectType,
                                          int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        netdata_fix_chart_name(windows_shared_buffer);
        struct net_framework_instances *p = dictionary_set(processes, windows_shared_buffer, NULL, sizeof(*p));

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRInteropCOMCallableWrappers)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_com_callable_wrappers", windows_shared_buffer);
            if (!p->st_clrinterop_com_callable_wrappers) {
                p->st_clrinterop_com_callable_wrappers = rrdset_create_localhost("netframework"
                                                                                 , windows_shared_buffer, NULL
                                                                                 , "interop"
                                                                                 , "netframework.clrinterop_com_callable_wrappers"
                                                                                 , "COM callable wrappers (CCW)"
                                                                                 , "ccw/s"
                                                                                 , PLUGIN_WINDOWS_NAME
                                                                                 , "PerflibNetFramework"
                                                                                 , PRIO_NETFRAMEWORK_CLR_INTEROP_CCW
                                                                                 , update_every
                                                                                 , RRDSET_TYPE_LINE
                                                                                 );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrinterop_com_callable_wrappers_total", windows_shared_buffer);
                p->rd_clrinterop_com_callable_wrappers  = rrddim_add(p->st_clrinterop_com_callable_wrappers,
                                                                    id,
                                                                    "com_callable_wrappers",
                                                                    1,
                                                                    1,
                                                                    RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrinterop_com_callable_wrappers->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrinterop_com_callable_wrappers,
                                  p->rd_clrinterop_com_callable_wrappers,
                                  (collected_number)p->NETFrameworkCLRInteropCOMCallableWrappers.current.Data);
            rrdset_done(p->st_clrinterop_com_callable_wrappers);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRInteropMarshalling)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_interop_marshalling", windows_shared_buffer);
            if (!p->st_clrinterop_marshalling) {
                p->st_clrinterop_marshalling = rrdset_create_localhost("netframework"
                                                                       , windows_shared_buffer, NULL
                                                                       , "interop"
                                                                       , "netframework.clrinterop_interop_marshallings"
                                                                       , "Arguments and return values marshallings"
                                                                       , "marshalling/s"
                                                                       , PLUGIN_WINDOWS_NAME
                                                                       , "PerflibNetFramework"
                                                                       , PRIO_NETFRAMEWORK_CLR_INTEROP_MARSHALLING
                                                                       , update_every
                                                                       , RRDSET_TYPE_LINE
                                                                       );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrinterop_interop_marshalling_total", windows_shared_buffer);
                p->rd_clrinterop_marshalling  = rrddim_add(p->st_clrinterop_marshalling,
                                                          id,
                                                          "marshallings",
                                                          1,
                                                          1,
                                                          RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrinterop_marshalling->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrinterop_marshalling,
                                  p->rd_clrinterop_marshalling,
                                  (collected_number)p->NETFrameworkCLRInteropMarshalling.current.Data);
            rrdset_done(p->st_clrinterop_marshalling);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRInteropStubsCreated)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrinterop_interop_stubs_created", windows_shared_buffer);
            if (!p->st_clrinterop_interop_stubs_created) {
                p->st_clrinterop_interop_stubs_created = rrdset_create_localhost("netframework"
                                                                                 , windows_shared_buffer, NULL
                                                                                 , "interop"
                                                                                 , "netframework.clrinterop_interop_stubs_created"
                                                                                 , "Created stubs"
                                                                                 , "stubs/s"
                                                                                 , PLUGIN_WINDOWS_NAME
                                                                                 , "PerflibNetFramework"
                                                                                 , PRIO_NETFRAMEWORK_CLR_INTEROP_STUBS_CREATED
                                                                                 , update_every
                                                                                 , RRDSET_TYPE_LINE
                                                                                 );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrinterop_interop_stubs_created_total", windows_shared_buffer);
                p->rd_clrinterop_interop_stubs_created  = rrddim_add(p->st_clrinterop_interop_stubs_created,
                                                                    id,
                                                                    "created",
                                                                    1,
                                                                    1,
                                                                    RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrinterop_interop_stubs_created->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrinterop_interop_stubs_created,
                                  p->rd_clrinterop_interop_stubs_created,
                                  (collected_number)p->NETFrameworkCLRInteropStubsCreated.current.Data);
            rrdset_done(p->st_clrinterop_interop_stubs_created);
        }
    }
}

static void netdata_framework_clr_jit(PERF_DATA_BLOCK *pDataBlock,
                                      PERF_OBJECT_TYPE *pObjectType,
                                      int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    char id[RRD_ID_LENGTH_MAX + 1];
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        netdata_fix_chart_name(windows_shared_buffer);
        struct net_framework_instances *p = dictionary_set(processes, windows_shared_buffer, NULL, sizeof(*p));

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRJITMethods)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_methods", windows_shared_buffer);
            if (!p->st_clrjit_methods) {
                p->st_clrjit_methods = rrdset_create_localhost("netframework"
                                                                                 , windows_shared_buffer, NULL
                                                                                 , "jit"
                                                                                 , "netframework.clrjit_methods"
                                                                                 , "JIT-compiled methods"
                                                                                 , "methods/s"
                                                                                 , PLUGIN_WINDOWS_NAME
                                                                                 , "PerflibNetFramework"
                                                                                 , PRIO_NETFRAMEWORK_CLR_JIT_METHODS
                                                                                 , update_every
                                                                                 , RRDSET_TYPE_LINE
                                                                                 );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrjit_methods_total", windows_shared_buffer);
                p->rd_clrjit_methods  = rrddim_add(p->st_clrjit_methods,
                                                  id,
                                                  "jit-compiled",
                                                  1,
                                                  1,
                                                  RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrjit_methods->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrjit_methods,
                                  p->rd_clrjit_methods,
                                  (collected_number)p->NETFrameworkCLRJITMethods.current.Data);
            rrdset_done(p->st_clrjit_methods);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRJITFrequencyTime) &&
            perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRJITPercentTime)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_time", windows_shared_buffer);
            if (!p->st_clrjit_time) {
                p->st_clrjit_time = rrdset_create_localhost("netframework"
                                                            , windows_shared_buffer, NULL
                                                            , "jit"
                                                            , "netframework.clrjit_time"
                                                            , "Time spent in JIT compilation"
                                                            , "percentage"
                                                            , PLUGIN_WINDOWS_NAME
                                                            , "PerflibNetFramework"
                                                            , PRIO_NETFRAMEWORK_CLR_JIT_TIME
                                                            , update_every
                                                            , RRDSET_TYPE_LINE
                                                            );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrjit_time_percent", windows_shared_buffer);
                p->rd_clrjit_time  = rrddim_add(p->st_clrjit_time,
                                                   id,
                                                   "time",
                                                   1,
                                                   100,
                                                   RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrjit_time->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            double percTime = (double) p->NETFrameworkCLRJITPercentTime.current.Data;
            percTime /= (double) p->NETFrameworkCLRJITFrequencyTime.current.Data;
            percTime *= 100;
            rrddim_set_by_pointer(p->st_clrjit_time,
                                  p->rd_clrjit_time,
                                  (collected_number) percTime);
            rrdset_done(p->st_clrjit_time);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRJITStandardFailures)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_standard_failures", windows_shared_buffer);
            if (!p->st_clrjit_standard_failures) {
                p->st_clrjit_standard_failures = rrdset_create_localhost("netframework"
                                                                         , windows_shared_buffer, NULL
                                                                         , "jit"
                                                                         , "netframework.clrjit_standard_failures"
                                                                         , "JIT compiler failures"
                                                                         , "failures/s"
                                                                         , PLUGIN_WINDOWS_NAME
                                                                         , "PerflibNetFramework"
                                                                         , PRIO_NETFRAMEWORK_CLR_JIT_STANDARD_FAILURES
                                                                         , update_every
                                                                         , RRDSET_TYPE_LINE
                                                                         );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrjit_standard_failures_total", windows_shared_buffer);
                p->rd_clrjit_standard_failures  = rrddim_add(p->st_clrjit_standard_failures,
                                                            id,
                                                            "failures",
                                                            1,
                                                            1,
                                                            RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrjit_standard_failures->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrjit_standard_failures,
                                  p->rd_clrjit_standard_failures,
                                  (collected_number)p->NETFrameworkCLRJITStandardFailures.current.Data);
            rrdset_done(p->st_clrjit_standard_failures);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRJITIlBytes)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrjit_il_bytes", windows_shared_buffer);
            if (!p->st_clrjit_il_bytes) {
                p->st_clrjit_il_bytes = rrdset_create_localhost("netframework"
                                                                , windows_shared_buffer, NULL
                                                                , "jit"
                                                                , "netframework.clrjit_il_bytes"
                                                                , "Compiled Microsoft intermediate language (MSIL) bytes"
                                                                , "bytes/s"
                                                                , PLUGIN_WINDOWS_NAME
                                                                , "PerflibNetFramework"
                                                                , PRIO_NETFRAMEWORK_CLR_JIT_IL_BYTES
                                                                , update_every
                                                                , RRDSET_TYPE_LINE
                                                                );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrjit_il_bytes_total", windows_shared_buffer);
                p->rd_clrjit_il_bytes  = rrddim_add(p->st_clrjit_il_bytes,
                                                   id,
                                                   "compiled_msil",
                                                   1,
                                                   1,
                                                   RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrjit_il_bytes->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrjit_il_bytes,
                                  p->rd_clrjit_il_bytes,
                                  (collected_number)p->NETFrameworkCLRJITIlBytes.current.Data);
            rrdset_done(p->st_clrjit_il_bytes);
        }
    }
}

static void netdata_framework_clr_loading(PERF_DATA_BLOCK *pDataBlock,
                                          PERF_OBJECT_TYPE *pObjectType,
                                          int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global_") == 0)
            continue;

        netdata_fix_chart_name(windows_shared_buffer);
        struct net_framework_instances *p = dictionary_set(processes, windows_shared_buffer, NULL, sizeof(*p));

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingHeapSize)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_loader_heap_size", windows_shared_buffer);
            if (!p->st_clrloading_heap_size) {
                p->st_clrloading_heap_size = rrdset_create_localhost("netframework"
                                                                     , windows_shared_buffer, NULL
                                                                     , "loading"
                                                                     , "netframework.clrloading_loader_heap_size"
                                                                     , "Memory committed by class loader"
                                                                     , "bytes"
                                                                     , PLUGIN_WINDOWS_NAME
                                                                     , "PerflibNetFramework"
                                                                     , PRIO_NETFRAMEWORK_CLR_LOADING_HEAP_SIZE
                                                                     , update_every
                                                                     , RRDSET_TYPE_LINE
                                                                     );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_loader_heap_size_bytes", windows_shared_buffer);
                p->rd_clrloading_heap_size  = rrddim_add(p->st_clrloading_heap_size,
                                                        id,
                                                        "committed",
                                                        1,
                                                        1,
                                                        RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_clrloading_heap_size->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrloading_heap_size,
                                  p->rd_clrloading_heap_size,
                                  (collected_number)p->NETFrameworkCLRLoadingHeapSize.current.Data);
            rrdset_done(p->st_clrloading_heap_size);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingAppDomainsLoaded)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_appdomains_loaded", windows_shared_buffer);
            if (!p->st_clrloading_app_domains_loaded) {
                p->st_clrloading_app_domains_loaded = rrdset_create_localhost("netframework"
                                                                              , windows_shared_buffer, NULL
                                                                              , "loading"
                                                                              , "netframework.clrloading_appdomains_loaded"
                                                                              , "Loaded application domains"
                                                                              , "domain/s"
                                                                              , PLUGIN_WINDOWS_NAME
                                                                              , "PerflibNetFramework"
                                                                              , PRIO_NETFRAMEWORK_CLR_LOADING_APP_DOMAINS_LOADED
                                                                              , update_every
                                                                              , RRDSET_TYPE_LINE
                                                                              );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_appdomains_loaded_total", windows_shared_buffer);
                p->rd_clrloading_app_domains_loaded  = rrddim_add(p->st_clrloading_app_domains_loaded,
                                                                 id,
                                                                 "loaded",
                                                                 1,
                                                                 1,
                                                                 RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrloading_app_domains_loaded->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrloading_app_domains_loaded,
                                  p->rd_clrloading_app_domains_loaded,
                                  (collected_number)p->NETFrameworkCLRLoadingAppDomainsLoaded.current.Data);
            rrdset_done(p->st_clrloading_app_domains_loaded);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingAppDomainsUnloaded)) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_appdomains_unloaded", windows_shared_buffer);
                if (!p->st_clrloading_app_domains_unloaded) {
                    p->st_clrloading_app_domains_unloaded = rrdset_create_localhost("netframework"
                                                                                    , windows_shared_buffer, NULL
                                                                                    , "loading"
                                                                                    , "netframework.clrloading_appdomains_unloaded"
                                                                                    , "Unloaded application domains"
                                                                                    , "domain/s"
                                                                                    , PLUGIN_WINDOWS_NAME
                                                                                    , "PerflibNetFramework"
                                                                                    , PRIO_NETFRAMEWORK_CLR_LOADING_APP_DOMAINS_UNLOADED
                                                                                    , update_every
                                                                                    , RRDSET_TYPE_LINE
                                                                                    );

                    snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_appdomains_unloaded_total", windows_shared_buffer);
                    p->rd_clrloading_app_domains_unloaded  = rrddim_add(p->st_clrloading_app_domains_unloaded,
                                                                       id,
                                                                       "unloaded",
                                                                       1,
                                                                       1,
                                                                       RRD_ALGORITHM_INCREMENTAL);

                    rrdlabels_add(p->st_clrloading_app_domains_unloaded->rrdlabels,
                                  "process",
                                  windows_shared_buffer,
                                  RRDLABEL_SRC_AUTO);
                }

                rrddim_set_by_pointer(p->st_clrloading_app_domains_unloaded,
                                      p->rd_clrloading_app_domains_unloaded,
                                      (collected_number)p->NETFrameworkCLRLoadingAppDomainsUnloaded.current.Data);
                rrdset_done(p->st_clrloading_app_domains_unloaded);
            }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingAssembliesLoaded)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_assemblies_loaded", windows_shared_buffer);
            if (!p->st_clrloading_assemblies_loaded) {
                p->st_clrloading_assemblies_loaded = rrdset_create_localhost("netframework"
                                                                             , windows_shared_buffer, NULL
                                                                             , "loading"
                                                                             , "netframework.clrloading_assemblies_loaded"
                                                                             , "Loaded assemblies"
                                                                             , "assemblies/s"
                                                                             , PLUGIN_WINDOWS_NAME
                                                                             , "PerflibNetFramework"
                                                                             , PRIO_NETFRAMEWORK_CLR_LOADING_ASSEMBLIES_LOADED
                                                                             , update_every
                                                                             , RRDSET_TYPE_LINE
                                                                             );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_assemblies_loaded_total", windows_shared_buffer);
                p->rd_clrloading_assemblies_loaded  = rrddim_add(p->st_clrloading_assemblies_loaded,
                                                                id,
                                                                "loaded",
                                                                1,
                                                                1,
                                                                RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrloading_assemblies_loaded->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrloading_assemblies_loaded,
                                  p->rd_clrloading_assemblies_loaded,
                                  (collected_number)p->NETFrameworkCLRLoadingAssembliesLoaded.current.Data);
            rrdset_done(p->st_clrloading_assemblies_loaded);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingClassesLoaded)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_classes_loaded", windows_shared_buffer);
            if (!p->st_clrloading_classes_loaded) {
                p->st_clrloading_classes_loaded = rrdset_create_localhost("netframework"
                                                                          , windows_shared_buffer, NULL
                                                                          , "loading"
                                                                          , "netframework.clrloading_classes_loaded"
                                                                          , "Loaded classes in all assemblies"
                                                                          , "classes/s"
                                                                          , PLUGIN_WINDOWS_NAME
                                                                          , "PerflibNetFramework"
                                                                          , PRIO_NETFRAMEWORK_CLR_LOADING_CLASSES_LOADED
                                                                          , update_every
                                                                          , RRDSET_TYPE_LINE
                                                                          );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_classes_loaded_total", windows_shared_buffer);
                p->rd_clrloading_classes_loaded  = rrddim_add(p->st_clrloading_classes_loaded,
                                                                 id,
                                                             "loaded",
                                                             1,
                                                             1,
                                                             RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrloading_classes_loaded->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrloading_classes_loaded,
                                  p->rd_clrloading_classes_loaded,
                                  (collected_number)p->NETFrameworkCLRLoadingClassesLoaded.current.Data);
            rrdset_done(p->st_clrloading_classes_loaded);
        }

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &p->NETFrameworkCLRLoadingClassLoadFailure)) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_clrloading_class_load_failure", windows_shared_buffer);
            if (!p->st_clrloading_class_load_failure) {
                p->st_clrloading_class_load_failure = rrdset_create_localhost("netframework"
                                                                              , windows_shared_buffer, NULL
                                                                              , "loading"
                                                                              , "netframework.clrloading_class_load_failures"
                                                                              , "Class load failures"
                                                                              , "failures/s"
                                                                              , PLUGIN_WINDOWS_NAME
                                                                              , "PerflibNetFramework"
                                                                              , PRIO_NETFRAMEWORK_CLR_LOADING_CLASS_LOAD_FAILURE
                                                                              , update_every
                                                                              , RRDSET_TYPE_LINE
                                                                              );

                snprintfz(id, RRD_ID_LENGTH_MAX, "netframework_%s_clrloading_class_load_failures_total", windows_shared_buffer);
                p->rd_clrloading_class_load_failure  = rrddim_add(p->st_clrloading_class_load_failure,
                                                                 id,
                                                                 "class_load",
                                                                 1,
                                                                 1,
                                                                 RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_clrloading_class_load_failure->rrdlabels,
                              "process",
                              windows_shared_buffer,
                              RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(p->st_clrloading_class_load_failure,
                                  p->rd_clrloading_class_load_failure,
                                  (collected_number)p->NETFrameworkCLRLoadingClassLoadFailure.current.Data);
            rrdset_done(p->st_clrloading_class_load_failure);
        }
    }
}

struct netdata_netframework_objects {
    char *object;
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
} netframewrk_obj[NETDATA_NETFRAMEWORK_END] = {
    {
        .fnct = netdata_framework_clr_exceptions,
        .object = ".NET CLR Exceptions"
    },
    {
        .fnct = netdata_framework_clr_interop,
        .object = ".NET CLR Interop"
    },
    {
        .fnct = netdata_framework_clr_jit,
        .object = ".NET CLR Jit"
    },
    {
        .fnct = netdata_framework_clr_loading,
        .object = ".NET CLR Loading"
    }
};

int do_PerflibNetFramework(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    int i;
    for (i = 0; i < NETDATA_NETFRAMEWORK_END; i++) {
        DWORD id = RegistryFindIDByName(netframewrk_obj[i].object);
        if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if(!pDataBlock) continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, netframewrk_obj[i].object);
        if(!pObjectType) continue;

        netframewrk_obj[i].fnct(pDataBlock, pObjectType, update_every);
    }

    return 0;
}
