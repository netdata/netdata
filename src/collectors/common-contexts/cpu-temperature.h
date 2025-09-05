// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CPU_TEMPERATURE_H
#define NETDATA_CPU_TEMPERATURE_H

#include "common-contexts.h"

static inline RRDSET *common_cpu_temperature(int update_every) {
    static RRDSET *st_cpu_temp = NULL;
    if (!st_cpu_temp) {
        st_cpu_temp = rrdset_create_localhost(
                "cpu",
                "temperature",
                NULL,
                "temperature",
                "cpu.temperature",
                "Core temperature",
                "Celcius",
                , _COMMON_PLUGIN_NAME
                , _COMMON_PLUGIN_MODULE_NAME
                NETDATA_CHART_PRIO_CPU_TEMPERATURE,
                update_every,
                RRDSET_TYPE_LINE);
    }

    return st_cpu_temp;
}

#endif //NETDATA_CPU_TEMPERATURE_H

