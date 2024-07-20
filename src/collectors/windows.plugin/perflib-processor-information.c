// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibProcesses"
#include "../common-contexts/common-contexts.h"

struct processor_info {
    RRDDIM *rd_cpu_frequency;
    char cpu_freq_id[16];

    COUNTER_DATA cpuFrequency;
};

static struct processor_info total = { 0 };

static void initialize_processor_info_keys(struct processor_info *p) {
    p->cpuFrequency.key = "Processor Frequency";
}

void dict_processor_info_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct processor_info *p = value;
    initialize_processor_info_keys(p);
}

static DICTIONARY *processors_info = NULL;

static void initialize(void) {
    initialize_processor_info_keys(&total);

    processors_info = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct processor_info));

    dictionary_register_insert_callback(processors_info, dict_processor_info_insert_cb, NULL);
}

static inline int cpu_dict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    RRDSET *cpufreq = data;
    struct processor_info *p = value;

    if (!p->rd_cpu_frequency)
        p->rd_cpu_frequency = rrddim_add(cpufreq, p->cpu_freq_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(cpufreq, p->rd_cpu_frequency, (collected_number)p->cpuFrequency.current.Data);

    return 1;
}

static void cpu_freq_windows(int update_every)
{
    RRDSET *cpufreq = common_cpu_cpufreq(update_every);
    dictionary_walkthrough_read(processors_info, cpu_dict_callback, cpufreq);
    rrdset_done(cpufreq);
}

static bool do_processors_info(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Processor Information");
    if(!pObjectType) return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    int count_cpus = 0;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if(!pi) break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        struct processor_info *p;
        int cpu = -1;
        if(strcasecmp(windows_shared_buffer, "_Total") == 0) {
            p = &total;
        } else {
            // Labels are named X,Y
            char *comma = strchr(windows_shared_buffer, ',');
            if (!comma)
                continue;

            if (!isdigit(*(++comma)))
                continue;

            p = dictionary_set(processors_info, comma, NULL, sizeof(*p));
            cpu = str2i(comma);
            snprintfz(p->cpu_freq_id, sizeof(p->cpu_freq_id), "cpu%d", cpu);
            count_cpus++;
        }

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->cpuFrequency);
    }

    if (count_cpus)
        cpu_freq_windows(update_every);

    return true;
}

int do_PerflibProcessorInfo(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Processor Information");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_processors_info(pDataBlock, update_every);

    return 0;
}
