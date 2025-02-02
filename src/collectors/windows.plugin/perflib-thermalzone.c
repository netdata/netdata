// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct thermal_zone {
    RRDSET *st;
    RRDDIM *rd;

    COUNTER_DATA thermalZoneTemperature;
};

static inline void initialize_thermal_zone_keys(struct thermal_zone *p)
{
    p->thermalZoneTemperature.key = "Temperature";
}

void dict_thermal_zone_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct thermal_zone *p = value;
    initialize_thermal_zone_keys(p);
}

static DICTIONARY *thermal_zones = NULL;

static void initialize(void)
{
    thermal_zones = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct thermal_zone));

    dictionary_register_insert_callback(thermal_zones, dict_thermal_zone_insert_cb, NULL);
}

static bool do_thermal_zones(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Thermal Zone Information");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        struct thermal_zone *p = dictionary_set(thermal_zones, windows_shared_buffer, NULL, sizeof(*p));

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->thermalZoneTemperature);

        // https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/design-guide
        if (!p->st) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "thermalzone_%s_temperature", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st = rrdset_create_localhost(
                "system",
                id,
                NULL,
                "thermalzone",
                "system.thermalzone_temperature",
                "Thermal zone temperature",
                "Celsius",
                PLUGIN_WINDOWS_NAME,
                "ThermalZone",
                NETDATA_CHART_PRIO_WINDOWS_THERMAL_ZONES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd = rrddim_add(p->st, id, "temperature", 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st->rrdlabels, "thermalzone", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        // Convert to Celsius before to plot
        NETDATA_DOUBLE kTemperature = (NETDATA_DOUBLE)p->thermalZoneTemperature.current.Data;
        kTemperature -= 273.15;

        rrddim_set_by_pointer(p->st, p->rd, (collected_number)kTemperature);
        rrdset_done(p->st);
    }

    return true;
}

int do_PerflibThermalZone(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Thermal Zone Information");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_thermal_zones(pDataBlock, update_every);

    return 0;
}
