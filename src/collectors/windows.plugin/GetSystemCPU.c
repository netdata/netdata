// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

int do_GetSystemCPU(int update_every, usec_t dt __maybe_unused)
{
    FILETIME idleTime, kernelTime, userTime;

    if (GetSystemTimes(&idleTime, &kernelTime, &userTime) == 0) {
        netdata_log_error("GetSystemTimes() failed.");
        return 1;
    }

    ULONGLONG idle = FileTimeToULL(idleTime);
    ULONGLONG kernel = FileTimeToULL(kernelTime);
    ULONGLONG user = FileTimeToULL(userTime);

    // kernel includes idle
    kernel -= idle;

    static RRDSET *st = NULL;
    static RRDDIM *rd_user = NULL, *rd_kernel = NULL, *rd_idle = NULL;
    if (!st) {
        st = rrdset_create_localhost(
            "system",
            "cpu",
            NULL,
            "cpu",
            "system.cpu",
            "Total CPU utilization",
            "percentage",
            PLUGIN_WINDOWS_NAME,
            "GetSystemTimes",
            NETDATA_CHART_PRIO_SYSTEM_CPU,
            update_every,
            RRDSET_TYPE_STACKED);

        rd_user = rrddim_add(st, "user", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        rd_kernel = rrddim_add(st, "system", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        rd_idle = rrddim_add(st, "idle", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
        rrddim_hide(st, "idle");
    }

    rrddim_set_by_pointer(st, rd_user, (collected_number)user);
    rrddim_set_by_pointer(st, rd_kernel, (collected_number)kernel);
    rrddim_set_by_pointer(st, rd_idle, (collected_number)idle);
    rrdset_done(st);

    return 0;
}
