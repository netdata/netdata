// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

#define _COMMON_PLUGIN_NAME "macos.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "power_sources"
#include "../common-contexts/common-contexts.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>
#include <IOKit/pwr_mgt/IOPM.h>

#define MACOS_POWER_SOURCE_NAME_MAX 128

struct macos_power_source {
    struct power_supply ps;
    struct simple_property capacity;
    struct simple_property voltage;

    bool seen;
    bool has_capacity;
    bool has_voltage;
    bool has_current;
    bool has_temperature;
    bool has_cycles;

    collected_number current_ma;
    collected_number temperature_mc;
    collected_number cycles;

    RRDSET *st_current;
    RRDDIM *rd_current;
    RRDSET *st_temperature;
    RRDDIM *rd_temperature;
    RRDSET *st_cycles;
    RRDDIM *rd_cycles;

    struct macos_power_source *next;
};

static struct macos_power_source *power_sources_root = NULL;

static bool cf_dictionary_get_int64(CFDictionaryRef dict, CFStringRef key, int64_t *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFNumberGetTypeID())
        return false;

    return CFNumberGetValue((CFNumberRef)obj, kCFNumberSInt64Type, value);
}

static bool cf_dictionary_get_bool(CFDictionaryRef dict, CFStringRef key, bool *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFBooleanGetTypeID())
        return false;

    *value = CFBooleanGetValue((CFBooleanRef)obj);
    return true;
}

static bool cf_dictionary_get_string(CFDictionaryRef dict, CFStringRef key, char *dst, size_t dst_size)
{
    if (!dict || !key || !dst || dst_size == 0)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFStringGetTypeID())
        return false;

    if (!CFStringGetCString((CFStringRef)obj, dst, dst_size, kCFStringEncodingUTF8))
        return false;

    dst[dst_size - 1] = '\0';
    return dst[0] != '\0';
}

static void macos_power_source_obsolete(struct macos_power_source *ps)
{
    if (ps->capacity.st)
        rrdset_is_obsolete___safe_from_collector_thread(ps->capacity.st);
    if (ps->voltage.st)
        rrdset_is_obsolete___safe_from_collector_thread(ps->voltage.st);
    if (ps->st_current)
        rrdset_is_obsolete___safe_from_collector_thread(ps->st_current);
    if (ps->st_temperature)
        rrdset_is_obsolete___safe_from_collector_thread(ps->st_temperature);
    if (ps->st_cycles)
        rrdset_is_obsolete___safe_from_collector_thread(ps->st_cycles);
}

static void macos_power_source_free(struct macos_power_source *ps)
{
    if (!ps)
        return;

    macos_power_source_obsolete(ps);
    freez(ps->ps.name);
    freez(ps);
}

static struct macos_power_source *macos_power_source_get_or_create(const char *name)
{
    for (struct macos_power_source *ps = power_sources_root; ps; ps = ps->next) {
        if (ps->ps.name && !strcmp(ps->ps.name, name))
            return ps;
    }

    struct macos_power_source *ps = callocz(1, sizeof(*ps));
    ps->ps.name = strdupz(name);
    ps->ps.hash = simple_hash(name);
    ps->ps.capacity = &ps->capacity;
    ps->capacity.fd = -1;
    ps->ps.next = NULL;
    ps->voltage.fd = -1;
    ps->next = power_sources_root;
    power_sources_root = ps;

    return ps;
}

static void macos_power_source_labels(RRDSET *st, const struct macos_power_source *ps)
{
    rrdlabels_add(st->rrdlabels, "device", ps->ps.name, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "source", "iokit", RRDLABEL_SRC_AUTO);
}

static void macos_power_source_update_current(struct macos_power_source *ps, int update_every)
{
    if (!ps->st_current) {
        ps->st_current = rrdset_create_localhost(
            "powersupply_current",
            ps->ps.name,
            NULL,
            "current",
            "powersupply.current",
            "Power Supply Current",
            "A",
            "macos.plugin",
            "power_sources",
            NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE + 1,
            update_every,
            RRDSET_TYPE_LINE);

        macos_power_source_labels(ps->st_current, ps);
    }

    if (!ps->rd_current)
        ps->rd_current = rrddim_add(ps->st_current, "current", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(ps->st_current, ps->rd_current, ps->current_ma);
    rrdset_done(ps->st_current);
}

static void macos_power_source_update_temperature(struct macos_power_source *ps, int update_every)
{
    if (!ps->st_temperature) {
        char chart_id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(chart_id, sizeof(chart_id), "macos_%s_battery_temperature", ps->ps.name);
        netdata_fix_chart_id(chart_id);

        ps->st_temperature = rrdset_create_localhost(
            "sensors",
            chart_id,
            NULL,
            "Battery",
            "system.hw.sensor.temperature.input",
            "Battery Temperature",
            "degrees Celsius",
            "macos.plugin",
            "power_sources",
            NETDATA_CHART_PRIO_SENSORS,
            update_every,
            RRDSET_TYPE_LINE);

        macos_power_source_labels(ps->st_temperature, ps);
        rrdlabels_add(ps->st_temperature->rrdlabels, "sensor", "battery_temperature", RRDLABEL_SRC_AUTO);
    }

    if (!ps->rd_temperature)
        ps->rd_temperature = rrddim_add(ps->st_temperature, "input", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(ps->st_temperature, ps->rd_temperature, ps->temperature_mc);
    rrdset_done(ps->st_temperature);
}

static void macos_power_source_update_cycles(struct macos_power_source *ps, int update_every)
{
    if (!ps->st_cycles) {
        ps->st_cycles = rrdset_create_localhost(
            "powersupply_cycles",
            ps->ps.name,
            NULL,
            "battery",
            "powersupply.cycles",
            "Battery Cycle Count",
            "cycles",
            "macos.plugin",
            "power_sources",
            NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE + 2,
            update_every,
            RRDSET_TYPE_LINE);

        macos_power_source_labels(ps->st_cycles, ps);
    }

    if (!ps->rd_cycles)
        ps->rd_cycles = rrddim_add(ps->st_cycles, "cycles", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(ps->st_cycles, ps->rd_cycles, ps->cycles);
    rrdset_done(ps->st_cycles);
}

static bool macos_power_source_should_collect(CFDictionaryRef desc)
{
    bool present = true;
    if (cf_dictionary_get_bool(desc, CFSTR(kIOPSIsPresentKey), &present) && !present)
        return false;

    char type[MACOS_POWER_SOURCE_NAME_MAX + 1] = "";
    if (cf_dictionary_get_string(desc, CFSTR(kIOPSTypeKey), type, sizeof(type)) &&
        strcmp(type, kIOPSInternalBatteryType) != 0 &&
        strcmp(type, kIOPSUPSType) != 0)
        return false;

    return true;
}

static void macos_power_source_update_from_description(struct macos_power_source *ps, CFDictionaryRef desc)
{
    int64_t current_capacity = 0, max_capacity = 0;
    if (cf_dictionary_get_int64(desc, CFSTR(kIOPSCurrentCapacityKey), &current_capacity) &&
        cf_dictionary_get_int64(desc, CFSTR(kIOPSMaxCapacityKey), &max_capacity) &&
        max_capacity > 0) {
        int64_t capacity = (current_capacity * 100 + max_capacity / 2) / max_capacity;
        if (capacity < 0)
            capacity = 0;
        else if (capacity > 100)
            capacity = 100;

        ps->capacity.value = (unsigned long long)capacity;
        ps->capacity.ok = true;
        ps->has_capacity = true;
    } else {
        ps->capacity.ok = false;
        ps->has_capacity = false;
    }

    int64_t voltage_mv = 0;
    if (cf_dictionary_get_int64(desc, CFSTR(kIOPSVoltageKey), &voltage_mv) && voltage_mv > 0) {
        ps->voltage.value = (unsigned long long)voltage_mv;
        ps->voltage.ok = true;
        ps->has_voltage = true;
    } else {
        ps->voltage.ok = false;
        ps->has_voltage = false;
    }

    int64_t current_ma = 0;
    ps->has_current = cf_dictionary_get_int64(desc, CFSTR(kIOPSCurrentKey), &current_ma);
    if (ps->has_current)
        ps->current_ma = (collected_number)current_ma;

    int64_t temperature_c = 0;
    ps->has_temperature = cf_dictionary_get_int64(desc, CFSTR(kIOPSTemperatureKey), &temperature_c);
    if (ps->has_temperature)
        ps->temperature_mc = (collected_number)(temperature_c * 1000);

    int64_t cycles = 0;
    ps->has_cycles =
        cf_dictionary_get_int64(desc, CFSTR(kIOPMPSCycleCountKey), &cycles) ||
        cf_dictionary_get_int64(desc, CFSTR(kIOBatteryCycleCountKey), &cycles);
    if (ps->has_cycles && cycles >= 0)
        ps->cycles = (collected_number)cycles;
    else
        ps->has_cycles = false;
}

static void macos_power_source_prune_missing(void)
{
    struct macos_power_source **pp = &power_sources_root;
    while (*pp) {
        struct macos_power_source *ps = *pp;
        if (!ps->seen) {
            *pp = ps->next;
            macos_power_source_free(ps);
        } else {
            pp = &ps->next;
        }
    }
}

int do_macos_power_sources(int update_every, usec_t dt __maybe_unused)
{
    static int do_capacity = -1, do_voltage = -1, do_current = -1, do_temperature = -1, do_cycles = -1;

    if (unlikely(do_capacity == -1)) {
        do_capacity = inicfg_get_boolean(&netdata_config, "plugin:macos:power_sources", "battery capacity", 1);
        do_voltage = inicfg_get_boolean(&netdata_config, "plugin:macos:power_sources", "power supply voltage", 1);
        do_current = inicfg_get_boolean(&netdata_config, "plugin:macos:power_sources", "power supply current", 1);
        do_temperature = inicfg_get_boolean(&netdata_config, "plugin:macos:power_sources", "battery temperature", 1);
        do_cycles = inicfg_get_boolean(&netdata_config, "plugin:macos:power_sources", "battery cycle count", 1);
    }

    for (struct macos_power_source *ps = power_sources_root; ps; ps = ps->next)
        ps->seen = false;

    CFTypeRef info = IOPSCopyPowerSourcesInfo();
    if (unlikely(!info)) {
        collector_error("MACOS: IOPSCopyPowerSourcesInfo() failed");
        return 1;
    }

    CFArrayRef list = IOPSCopyPowerSourcesList(info);
    if (unlikely(!list)) {
        CFRelease(info);
        collector_error("MACOS: IOPSCopyPowerSourcesList() failed");
        return 1;
    }

    CFIndex count = CFArrayGetCount(list);
    if (count > 32)
        count = 32;

    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef source = CFArrayGetValueAtIndex(list, i);
        CFDictionaryRef desc = IOPSGetPowerSourceDescription(info, source);
        if (!desc || CFGetTypeID(desc) != CFDictionaryGetTypeID() || !macos_power_source_should_collect(desc))
            continue;

        char name[MACOS_POWER_SOURCE_NAME_MAX + 1] = "";
        if (!cf_dictionary_get_string(desc, CFSTR(kIOPSNameKey), name, sizeof(name)))
            snprintfz(name, sizeof(name), "PowerSource%ld", (long)i + 1);

        netdata_fix_chart_id(name);
        struct macos_power_source *ps = macos_power_source_get_or_create(name);
        ps->seen = true;

        macos_power_source_update_from_description(ps, desc);

        bool add_capacity_source_label = !ps->capacity.st;
        if (do_capacity && ps->has_capacity)
            rrdset_create_simple_prop(
                &ps->ps,
                &ps->capacity,
                "Battery Capacity",
                "capacity",
                1,
                "percentage",
                NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
                update_every);
        if (add_capacity_source_label && ps->capacity.st)
            rrdlabels_add(ps->capacity.st->rrdlabels, "source", "iokit", RRDLABEL_SRC_AUTO);

        bool add_voltage_source_label = !ps->voltage.st;
        if (do_voltage && ps->has_voltage)
            rrdset_create_simple_prop(
                &ps->ps,
                &ps->voltage,
                "Power Supply Voltage",
                "voltage",
                1000,
                "V",
                NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE,
                update_every);
        if (add_voltage_source_label && ps->voltage.st)
            rrdlabels_add(ps->voltage.st->rrdlabels, "source", "iokit", RRDLABEL_SRC_AUTO);

        if (do_current && ps->has_current)
            macos_power_source_update_current(ps, update_every);

        if (do_temperature && ps->has_temperature)
            macos_power_source_update_temperature(ps, update_every);

        if (do_cycles && ps->has_cycles)
            macos_power_source_update_cycles(ps, update_every);
    }

    CFRelease(list);
    CFRelease(info);

    macos_power_source_prune_missing();

    return 0;
}
