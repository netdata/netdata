// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_freebsd.h"

#define _COMMON_PLUGIN_NAME "freebsd.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "hw.acpi.battery"
#include "../common-contexts/common-contexts.h"

// --------------------------------------------------------------------------------------------------------------------
// hw.acpi.battery

// The kernel aggregates every ACPI battery into these sysctls and reports -1 when no battery has valid data.
#define ACPI_BATTERY_UNKNOWN (-1)

// Charts aggregate ACPI battery capacity; returns 1 to disable the module when the battery driver is absent.
int do_hw_acpi_battery(int update_every, usec_t dt) {
    (void)dt;
    static int mib_units[4] = {0, 0, 0, 0}, mib_life[4] = {0, 0, 0, 0};
    static char name[] = "acpi_battery";
    static struct simple_property capacity = { 0 };
    static struct power_supply ps = { .name = name, .capacity = &capacity };
    int units = 0, life = ACPI_BATTERY_UNKNOWN;

    // The node exists only when the ACPI battery driver attached; without it there is nothing to collect.
    if (unlikely(GETSYSCTL_SIMPLE("hw.acpi.battery.units", mib_units, units) ||
                 GETSYSCTL_SIMPLE("hw.acpi.battery.life", mib_life, life))) {
        collector_error("DISABLED: powersupply.capacity chart");
        collector_error("DISABLED: hw.acpi.battery module");
        return 1;
    }

    // No battery units means the batteries are known to be gone (e.g. detached), not a failed read.
    if (unlikely(units <= 0)) {
        if (capacity.st) {
            rrdset_is_obsolete___safe_from_collector_thread(capacity.st);
            capacity.st = NULL;
            capacity.rd = NULL;
        }
        return 0;
    }

    // An unknown reading is a gap, not 0%: finish the iteration without setting the dimension so an empty point is
    // stored, instead of skipping it and letting a short outage be interpolated from the last value.
    if (unlikely(life == ACPI_BATTERY_UNKNOWN)) {
        if (capacity.st)
            rrdset_done(capacity.st);
        return 0;
    }

    capacity.value = (unsigned long long)life;
    rrdset_create_simple_prop(
        &ps,
        &capacity,
        "Battery Capacity",
        "capacity",
        1,
        "percentage",
        NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
        update_every);

    return 0;
}
