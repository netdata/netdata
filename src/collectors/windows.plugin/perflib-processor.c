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

struct processor_topology_entry {
    uint32_t numa_node;
    uint32_t numa_index;
    int cpu_id;
};

struct processor_topology {
    struct processor_topology_entry *entries;
    size_t entries_count;
    size_t entries_capacity;
};

static struct processor_topology processor_topology = {0};

// Windows 20H2 extends the SDK's NUMA_NODE_RELATIONSHIP with GroupCount and GroupMasks.
struct processor_numa_node_relationship {
    DWORD node_number;
    BYTE reserved[18];
    WORD group_count;
    GROUP_AFFINITY group_masks[ANYSIZE_ARRAY];
};

static void initialize(void)
{
    initialize_processor_keys(&total);

    processors = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct processor));

    dictionary_register_insert_callback(processors, dict_processor_insert_cb, NULL);
}

static unsigned int processor_mask_span(KAFFINITY mask)
{
    unsigned int span = 0;
    while (mask) {
        span++;
        mask >>= 1;
    }

    return span;
}

static bool
processor_topology_add(struct processor_topology *topology, uint32_t numa_node, uint32_t numa_index, uint64_t cpu_id)
{
    if (cpu_id > INT_MAX)
        return false;

    if (topology->entries_count == topology->entries_capacity) {
        size_t capacity = topology->entries_capacity ? topology->entries_capacity * 2 : 64;
        topology->entries = reallocz(topology->entries, capacity * sizeof(*topology->entries));
        topology->entries_capacity = capacity;
    }

    topology->entries[topology->entries_count++] = (struct processor_topology_entry){
        .numa_node = numa_node,
        .numa_index = numa_index,
        .cpu_id = (int)cpu_id,
    };
    return true;
}

static bool processor_topology_add_mask(
    struct processor_topology *topology,
    uint32_t numa_node,
    KAFFINITY mask,
    uint64_t group_offset)
{
    uint32_t numa_index = 0;
    for (size_t i = 0; i < topology->entries_count; i++) {
        if (topology->entries[i].numa_node == numa_node)
            numa_index++;
    }

    for (unsigned int bit = 0; mask; bit++, mask >>= 1) {
        if (!(mask & 1))
            continue;

        if (!processor_topology_add(topology, numa_node, numa_index++, group_offset + bit))
            return false;
    }

    return true;
}

static bool processor_topology_from_extended_api(struct processor_topology *topology, bool *api_available)
{
    typedef BOOL(WINAPI * get_logical_processor_information_ex_t)(
        LOGICAL_PROCESSOR_RELATIONSHIP relationship_type,
        PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX buffer,
        PDWORD returned_length);

    *api_available = false;

    HMODULE kernel32 = GetModuleHandleA("kernel32.dll");
    union {
        FARPROC raw;
        get_logical_processor_information_ex_t typed;
    } api = {.raw = kernel32 ? GetProcAddress(kernel32, "GetLogicalProcessorInformationEx") : NULL};
    get_logical_processor_information_ex_t get_logical_processor_information_ex = api.typed;
    if (!get_logical_processor_information_ex)
        return false;

    *api_available = true;

    DWORD buffer_length = 0;
    if (get_logical_processor_information_ex(RelationAll, NULL, &buffer_length) ||
        GetLastError() != ERROR_INSUFFICIENT_BUFFER)
        return false;

    PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX buffer = mallocz(buffer_length);
    if (!get_logical_processor_information_ex(RelationAll, buffer, &buffer_length)) {
        freez(buffer);
        return false;
    }

    uint64_t *group_offsets = NULL;
    WORD group_count = 0;
    BYTE *end = (BYTE *)buffer + buffer_length;
    for (PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX entry = buffer; (BYTE *)entry < end;
         entry = (PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX)((BYTE *)entry + entry->Size)) {
        if (entry->Relationship != RelationGroup)
            continue;

        size_t group_header_size =
            offsetof(SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX, Group) + offsetof(GROUP_RELATIONSHIP, GroupInfo);
        if (entry->Size < group_header_size || (BYTE *)entry + entry->Size > end)
            goto cleanup;

        group_count = entry->Group.ActiveGroupCount;
        if (!group_count || (size_t)(entry->Size - group_header_size) / sizeof(*entry->Group.GroupInfo) < group_count)
            goto cleanup;

        group_offsets = callocz(group_count, sizeof(*group_offsets));
        for (WORD group = 1; group < group_count; group++)
            group_offsets[group] =
                group_offsets[group - 1] + processor_mask_span(entry->Group.GroupInfo[group - 1].ActiveProcessorMask);

        break;
    }

    if (!group_offsets)
        goto cleanup;

    for (PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX entry = buffer; (BYTE *)entry < end;
         entry = (PSYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX)((BYTE *)entry + entry->Size)) {
        if (entry->Relationship != RelationNumaNode)
            continue;

        size_t numa_header_size = offsetof(SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX, NumaNode) +
                                  offsetof(struct processor_numa_node_relationship, group_masks);
        if (entry->Size < numa_header_size || (BYTE *)entry + entry->Size > end)
            goto cleanup;

        const struct processor_numa_node_relationship *numa = (const void *)&entry->NumaNode;
        WORD node_group_count = numa->group_count;
        if (!node_group_count)
            node_group_count = 1;
        if ((size_t)(entry->Size - numa_header_size) / sizeof(*numa->group_masks) < node_group_count)
            goto cleanup;

        for (WORD i = 0; i < node_group_count; i++) {
            const GROUP_AFFINITY *group_mask = &numa->group_masks[i];
            if (group_mask->Group >= group_count ||
                !processor_topology_add_mask(
                    topology, numa->node_number, group_mask->Mask, group_offsets[group_mask->Group]))
                goto cleanup;
        }
    }

    freez(group_offsets);
    freez(buffer);
    return topology->entries_count != 0;

cleanup:
    freez(group_offsets);
    freez(buffer);
    freez(topology->entries);
    *topology = (struct processor_topology){0};
    return false;
}

static bool processor_topology_from_legacy_api(struct processor_topology *topology)
{
    ULONG highest_node;
    if (!GetNumaHighestNodeNumber(&highest_node))
        return false;

    for (ULONG node = 0; node <= highest_node; node++) {
        ULONGLONG mask;
        if (!GetNumaNodeProcessorMask((UCHAR)node, &mask) || !mask)
            continue;

        if (!processor_topology_add_mask(topology, node, (KAFFINITY)mask, 0)) {
            freez(topology->entries);
            *topology = (struct processor_topology){0};
            return false;
        }
    }

    return topology->entries_count != 0;
}

static bool processor_topology_refresh(void)
{
    struct processor_topology topology = {0};
    bool extended_api_available = false;
    if (!processor_topology_from_extended_api(&topology, &extended_api_available)) {
        // The legacy API can fabricate duplicate IDs when the extended API is available.
        if (extended_api_available || !processor_topology_from_legacy_api(&topology))
            return false;
    }

    freez(processor_topology.entries);
    processor_topology = topology;
    return true;
}

static bool processor_topology_find(
    const struct processor_topology *topology, uint32_t numa_node, uint32_t numa_index, int *cpu_id)
{
    for (size_t i = 0; i < topology->entries_count; i++) {
        const struct processor_topology_entry *entry = &topology->entries[i];
        if (entry->numa_node == numa_node && entry->numa_index == numa_index) {
            *cpu_id = entry->cpu_id;
            return true;
        }
    }

    return false;
}

static bool processor_information_instance_to_cpu_id(
    const struct processor_topology *topology, const char *instance_name, int *cpu_id);

int perflib_processor_unittest(void)
{
    struct processor_topology topology = {0};
    const KAFFINITY first_group_mask = ((KAFFINITY)1 << 1) | ((KAFFINITY)1 << 3);
    const KAFFINITY second_group_mask = (KAFFINITY)1 << 1;
    const uint64_t second_group_offset = processor_mask_span(first_group_mask);
    int mapped_cpu = -1;
    int errors = 0;

    if (!processor_topology_add_mask(&topology, 0, first_group_mask, 0) ||
        !processor_topology_add_mask(&topology, 1, second_group_mask, second_group_offset) ||
        topology.entries_count != 3 || topology.entries[0].numa_index != 0 || topology.entries[0].cpu_id != 1 ||
        topology.entries[1].numa_index != 1 || topology.entries[1].cpu_id != 3 || topology.entries[2].numa_index != 0 ||
        topology.entries[2].cpu_id != 5 ||
        !(processor_topology_find(&topology, 1, 0, &mapped_cpu) && mapped_cpu == 5) ||
        !(processor_information_instance_to_cpu_id(&topology, "1,0", &mapped_cpu) && mapped_cpu == 5) ||
        processor_information_instance_to_cpu_id(&topology, "0,_Total", &mapped_cpu)) {
        fprintf(stderr, "perflib processor unittest: sparse processor masks produced invalid NUMA mapping\n");
        errors++;
    }

    freez(topology.entries);
    return errors;
}

// Processor Information instances use the PerfLib format NumaNode,NumaIndex.
static bool processor_information_instance_is_numa_total(const char *instance_name)
{
    const char *separator = strrchr(instance_name, ',');
    return separator && strcasecmp(separator + 1, "_Total") == 0;
}

static bool processor_information_instance_is_total(const char *instance_name)
{
    return strcasecmp(instance_name, "_Total") == 0 || processor_information_instance_is_numa_total(instance_name);
}

static bool processor_information_instance_to_cpu_id(
    const struct processor_topology *topology, const char *instance_name, int *cpu_id)
{
    char *separator;
    uint32_t numa_node = str2uint32_t(instance_name, &separator);
    if (separator == instance_name || *separator != ',')
        return false;

    const char *processor_number = separator + 1;
    if (processor_information_instance_is_total(instance_name))
        return false;

    char *end;
    uint32_t processor = str2uint32_t(processor_number, &end);
    if (end == processor_number || *end != '\0')
        return false;

    return processor_topology_find(topology, numa_node, processor, cpu_id);
}

static bool
do_processors(PERF_DATA_BLOCK *pDataBlock, const char *object_name, bool processor_information, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, object_name);
    if (!ObjectTypeHasInstances(pDataBlock, pObjectType))
        return false;

    static const RRDVAR_ACQUIRED *cpus_var = NULL;
    int cores_found = 0;
    uint64_t totalIPC = 0;
    bool topology_refresh_attempted = false;
    bool topology_mapping_failed = false;

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
        if (processor_information && processor_information_instance_is_numa_total(windows_shared_buffer))
            continue;

        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            p = &total;
            is_total = true;
            cpu = -1;
        } else {
            if (processor_information &&
                !processor_information_instance_to_cpu_id(&processor_topology, windows_shared_buffer, &cpu)) {
                if (!topology_refresh_attempted) {
                    topology_refresh_attempted = true;
                    processor_topology_refresh();
                }

                if (!processor_information_instance_to_cpu_id(&processor_topology, windows_shared_buffer, &cpu)) {
                    topology_mapping_failed = true;
                    continue;
                }
            }

            p = dictionary_set(processors, windows_shared_buffer, NULL, sizeof(*p));
            is_total = false;
            if (!processor_information)
                cpu = str2i(windows_shared_buffer);
            snprintfz(windows_shared_buffer, sizeof(windows_shared_buffer), "cpu%d", cpu);

            cores_found++;
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

    if (topology_mapping_failed) {
        static bool logged = false;
        if (!logged) {
            netdata_log_error("Processor Information topology mapping failed; skipping unmappable processor instances");
            logged = true;
        }
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

    const char *object_name = "Processor Information";
    DWORD id = RegistryFindIDByName(object_name);
    bool processor_information = id != PERFLIB_REGISTRY_NAME_NOT_FOUND;

    if (!processor_information) {
        object_name = "Processor";
        id = RegistryFindIDByName(object_name);
    }

    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (pDataBlock && do_processors(pDataBlock, object_name, processor_information, update_every))
        return 0;

    if (!processor_information)
        return 0;

    static bool fallback_logged = false;
    if (!fallback_logged) {
        netdata_log_error("Processor Information is unavailable in performance data; falling back to Processor");
        fallback_logged = true;
    }

    id = RegistryFindIDByName("Processor");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return 0;

    pDataBlock = perflibGetPerformanceData(id);
    if (pDataBlock)
        do_processors(pDataBlock, "Processor", false, update_every);

    return 0;
}
