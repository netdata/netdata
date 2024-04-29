// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

int do_GetSystemRAM(int update_every, usec_t dt __maybe_unused) {
    MEMORYSTATUSEX memStat = { 0 };
    memStat.dwLength = sizeof(memStat);

    if (!GlobalMemoryStatusEx(&memStat)) {
        netdata_log_error("GlobalMemoryStatusEx() failed.");
        return 1;
    }

    ULONGLONG totalMemory = memStat.ullTotalPhys;
    ULONGLONG freeMemory = memStat.ullAvailPhys;
    ULONGLONG usedMemory = totalMemory - freeMemory;

    static RRDSET *st = NULL;
    static RRDDIM *rd_free = NULL, *rd_used = NULL;
    if (!st) {
        st = rrdset_create_localhost(
            "system"
            , "ram"
            , NULL
            , "memory"
            , "system.ram"
            , "Total RAM utilization"
            , "MiB"
            , PLUGIN_WINDOWS_NAME
            , "GlobalMemoryStatusEx"
            , NETDATA_CHART_PRIO_SYSTEM_RAM
            , update_every
            , RRDSET_TYPE_STACKED
        );

        rd_free = rrddim_add(st, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rd_used = rrddim_add(st, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st, rd_free, (collected_number)freeMemory);
    rrddim_set_by_pointer(st, rd_used, (collected_number)usedMemory);
    rrdset_done(st);

    return 0;
}
