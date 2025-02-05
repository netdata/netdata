// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

int do_GetSystemUptime(int update_every, usec_t dt __maybe_unused)
{
    ULONGLONG uptime = GetTickCount64(); // in milliseconds

    static RRDSET *st = NULL;
    static RRDDIM *rd_uptime = NULL;
    if (!st) {
        st = rrdset_create_localhost(
            "system",
            "uptime",
            NULL,
            "uptime",
            "system.uptime",
            "System Uptime",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "GetSystemUptime",
            NETDATA_CHART_PRIO_SYSTEM_UPTIME,
            update_every,
            RRDSET_TYPE_LINE);

        rd_uptime = rrddim_add(st, "uptime", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st, rd_uptime, (collected_number)uptime);
    rrdset_done(st);

    return 0;
}
