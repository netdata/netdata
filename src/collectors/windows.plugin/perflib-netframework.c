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

    COUNTER_DATA NETFrameworkCLRExceptionThrown;
    COUNTER_DATA NETFrameworkCLRExceptionFilters;
    COUNTER_DATA NETFrameworkCLRExceptionFinallys;
    COUNTER_DATA NETFrameworkCLRExceptionTotalCatchDepth;

    COUNTER_DATA NETFrameworkCLRInteropCOMCallableWrappers;
    COUNTER_DATA NETFrameworkCLRInteropMarshalling;
    COUNTER_DATA NETFrameworkCLRInteropStubsCreated;
};

static inline void initialize_net_framework_processes_keys(struct net_framework_instances *p) {
    p->NETFrameworkCLRExceptionFilters.key = "# of Filters / sec";
    p->NETFrameworkCLRExceptionFinallys.key = "# of Finallys / sec";
    p->NETFrameworkCLRExceptionThrown.key = "# of Exceps Thrown / sec";
    p->NETFrameworkCLRExceptionTotalCatchDepth.key = "Throw To Catch Depth / sec";

    p->NETFrameworkCLRInteropCOMCallableWrappers.key = "# of CCWs";
    p->NETFrameworkCLRInteropMarshalling.key = "# of Stubs";
    p->NETFrameworkCLRInteropStubsCreated.key = "# of marshalling";
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
                                             char *object_name,
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

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
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
                                          char *object_name,
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

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
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
                                      char *object_name,
                                      int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

static void netdata_framework_clr_loading(PERF_DATA_BLOCK *pDataBlock,
                                          PERF_OBJECT_TYPE *pObjectType,
                                          char *object_name,
                                          int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

struct netdata_netframework_objects {
    char *object;
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, char *, int);
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

        netframewrk_obj[i].fnct(pDataBlock, pObjectType, netframewrk_obj[i].object, update_every);
    }

    return 0;
}
