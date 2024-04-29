// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

int do_GetSystemCPU(int update_every, usec_t dt __maybe_unused) {
    FILETIME idleTime, kernelTime, userTime;

    memset(&idleTime, 0, sizeof(idleTime));
    memset(&kernelTime, 0, sizeof(kernelTime));
    memset(&userTime, 0, sizeof(userTime));

    if(GetSystemTimes(&idleTime, &kernelTime, &userTime) == 0) {
        netdata_log_error("GetSystemTimes() failed.");
        return 1;
    }

    static ULONGLONG lastIdle = 0, lastKernel = 0, lastUser = 0;

    ULONGLONG idle = FileTimeToULL(idleTime);
    ULONGLONG kernel = FileTimeToULL(kernelTime);
    ULONGLONG user = FileTimeToULL(userTime);

    ULONGLONG diffIdle = idle - lastIdle;
    ULONGLONG diffUser = user - lastUser;
    ULONGLONG diffKernel = kernel - lastKernel;

    lastIdle = idle;
    lastUser = user;
    lastKernel = kernel;

    ULONGLONG total = diffKernel + diffUser;
    ULONGLONG used = total - diffIdle;

    ULONGLONG finalKernel = diffKernel * used / total;
    ULONGLONG finalUser = diffUser * used / total;

    static RRDSET *st = NULL;
    static RRDDIM *rd_user = NULL, *rd_kernel = NULL, *rd_idle = NULL;
    if(!st) {
        st = rrdset_create_localhost(
            "system"
            , "cpu"
            , NULL
            , "cpu"
            , "system.cpu"
            , "Total CPU utilization"
            , "percentage"
            , PLUGIN_WINDOWS_NAME
            , "GetSystemTimes"
            , NETDATA_CHART_PRIO_SYSTEM_CPU
            , update_every
            , RRDSET_TYPE_STACKED
        );

        rd_user = rrddim_add(st, "user", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
        rd_kernel = rrddim_add(st, "kernel", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
        rd_idle = rrddim_add(st, "idle", NULL, 1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
        rrddim_hide(st, "idle");
    }

    rrddim_set_by_pointer(st, rd_user, (collected_number )finalUser);
    rrddim_set_by_pointer(st, rd_kernel, (collected_number )finalKernel);
    rrddim_set_by_pointer(st, rd_idle, (collected_number )diffIdle);
    rrdset_done(st);

    return 0;
}
