// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibProcesses"
#include "../common-contexts/common-contexts.h"

struct processor {
    bool collected_metadata;

    RRDSET *st;
    RRDDIM *rd_user;
    RRDDIM *rd_system;
    RRDDIM *rd_irq;
    RRDDIM *rd_dpc;
    RRDDIM *rd_idle;

    //    RRDSET *st2;
    //    RRDDIM *rd2_busy;

    COUNTER_DATA percentProcessorTime;
    COUNTER_DATA percentUserTime;
    COUNTER_DATA percentPrivilegedTime;
    COUNTER_DATA percentDPCTime;
    COUNTER_DATA percentInterruptTime;
    COUNTER_DATA percentIdleTime;

    COUNTER_DATA interruptsPerSec;
};

struct processor total = {0};

void initialize_processor_keys(struct processor *p)
{
    p->percentProcessorTime.key = "% Processor Time";
    p->percentUserTime.key = "% User Time";
    p->percentPrivilegedTime.key = "% Privileged Time";
    p->percentDPCTime.key = "% DPC Time";
    p->percentInterruptTime.key = "% Interrupt Time";
    p->percentIdleTime.key = "% Idle Time";
    p->interruptsPerSec.key = "Interrupts/sec";
}

void dict_processor_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct processor *p = value;
    initialize_processor_keys(p);
}

static DICTIONARY *processors = NULL;

static void initialize(void)
{
    initialize_processor_keys(&total);

    processors = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct processor));

    dictionary_register_insert_callback(processors, dict_processor_insert_cb, NULL);
}

static bool do_processors(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Processor");
    if (!pObjectType)
        return false;

    static const RRDVAR_ACQUIRED *cpus_var = NULL;
    int cores_found = 0;
    uint64_t totalIPC = 0;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        bool is_total = false;
        struct processor *p;
        int cpu = -1;
        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            p = &total;
            is_total = true;
            cpu = -1;
        } else {
            p = dictionary_set(processors, windows_shared_buffer, NULL, sizeof(*p));
            is_total = false;
            cpu = str2i(windows_shared_buffer);
            snprintfz(windows_shared_buffer, sizeof(windows_shared_buffer), "cpu%d", cpu);

            if (cpu + 1 > cores_found)
                cores_found = cpu + 1;
        }

        if (!is_total && !p->collected_metadata) {
            // TODO collect processor metadata
            p->collected_metadata = true;
        }

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentProcessorTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentUserTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentPrivilegedTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentDPCTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentInterruptTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->percentIdleTime);

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->interruptsPerSec);

        if (!p->st) {
            p->st = rrdset_create_localhost(
                is_total ? "system" : "cpu",
                is_total ? "cpu" : windows_shared_buffer,
                NULL,
                is_total ? "cpu" : "utilization",
                is_total ? "system.cpu" : "cpu.cpu",
                is_total ? "Total CPU Utilization" : "Core Utilization",
                "percentage",
                PLUGIN_WINDOWS_NAME,
                "PerflibProcessor",
                is_total ? NETDATA_CHART_PRIO_SYSTEM_CPU : NETDATA_CHART_PRIO_CPU_PER_CORE,
                update_every,
                RRDSET_TYPE_STACKED);

            p->rd_irq = rrddim_add(p->st, "interrupts", "irq", 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            p->rd_user = rrddim_add(p->st, "user", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            p->rd_system = rrddim_add(p->st, "privileged", "system", 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            p->rd_dpc = rrddim_add(p->st, "dpc", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            p->rd_idle = rrddim_add(p->st, "idle", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
            rrddim_hide(p->st, "idle");

            if (!is_total)
                rrdlabels_add(p->st->rrdlabels, "cpu", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            else
                cpus_var = rrdvar_host_variable_add_and_acquire(localhost, "active_processors");
        }

        uint64_t user = p->percentUserTime.current.Data;
        uint64_t system = p->percentPrivilegedTime.current.Data;
        uint64_t dpc = p->percentDPCTime.current.Data;
        uint64_t irq = p->percentInterruptTime.current.Data;
        uint64_t idle = p->percentIdleTime.current.Data;

        totalIPC += p->interruptsPerSec.current.Data;

        rrddim_set_by_pointer(p->st, p->rd_user, (collected_number)user);
        rrddim_set_by_pointer(p->st, p->rd_system, (collected_number)system);
        rrddim_set_by_pointer(p->st, p->rd_irq, (collected_number)irq);
        rrddim_set_by_pointer(p->st, p->rd_dpc, (collected_number)dpc);
        rrddim_set_by_pointer(p->st, p->rd_idle, (collected_number)idle);
        rrdset_done(p->st);

        //        if(!p->st2) {
        //            p->st2 = rrdset_create_localhost(
        //                is_total ? "system" : "cpu2"
        //                , is_total ? "cpu3" : buffer
        //                , NULL
        //                , is_total ? "utilization" : buffer
        //                , is_total ? "system.cpu3" : "cpu2.cpu"
        //                , is_total ? "Total CPU Utilization" : "Core Utilization"
        //                , "percentage"
        //                , PLUGIN_WINDOWS_NAME
        //                , "PerflibProcessor"
        //                , is_total ? NETDATA_CHART_PRIO_SYSTEM_CPU : NETDATA_CHART_PRIO_CPU_PER_CORE
        //                , update_every
        //                , RRDSET_TYPE_STACKED
        //            );
        //
        //            p->rd2_busy = perflib_rrddim_add(p->st2, "busy", NULL, 1, 1, &p->percentProcessorTime);
        //            rrddim_hide(p->st2, "idle");
        //
        //            if(!is_total)
        //                rrdlabels_add(p->st->rrdlabels, "cpu", buffer, RRDLABEL_SRC_AUTO);
        //        }
        //
        //        perflib_rrddim_set_by_pointer(p->st2, p->rd2_busy, &p->percentProcessorTime);
        //        rrdset_done(p->st2);
    }

    if (cpus_var)
        rrdvar_host_variable_set(localhost, cpus_var, cores_found);

    common_interrupts(totalIPC, update_every, NULL);

    return true;
}

int do_PerflibProcessor(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Processor");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_processors(pDataBlock, update_every);

    return 0;
}
