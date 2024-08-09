// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CPU_CPUFREQ_H
#define NETDATA_CPU_CPUFREQ_H

#include "common-contexts.h"

static inline RRDSET * common_cpu_cpufreq(int update_every) {
    static RRDSET *st_scaling_cur_freq = NULL;
    if(unlikely(!st_scaling_cur_freq)) {
        st_scaling_cur_freq = rrdset_create_localhost(
            "cpu"
        , "cpufreq"
        , NULL
        , "cpufreq"
        , "cpufreq.cpufreq"
        , "Current CPU Frequency"
        , "MHz"
        , _COMMON_PLUGIN_NAME
        , _COMMON_PLUGIN_MODULE_NAME
        , NETDATA_CHART_PRIO_CPUFREQ_SCALING_CUR_FREQ
        , update_every
        , RRDSET_TYPE_LINE
        );
    }

    return st_scaling_cur_freq;
}

#endif //NETDATA_CPU_CPUFREQ_H

