// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

int do_proc_uptime(int update_every, usec_t dt) {
    (void)dt;

    static const char *uptime_filename = NULL;
    if(!uptime_filename) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/uptime");

        uptime_filename = inicfg_get(&netdata_config, "plugin:proc:/proc/uptime", "filename to monitor", filename);
    }

    static RRDSET *st = NULL;
    static RRDDIM *rd = NULL;

    if(unlikely(!st)) {

        st = rrdset_create_localhost(
                "system"
                , "uptime"
                , NULL
                , "uptime"
                , NULL
                , "System Uptime"
                , "seconds"
                , PLUGIN_PROC_NAME
                , "/proc/uptime"
                , NETDATA_CHART_PRIO_SYSTEM_UPTIME
                , update_every
                , RRDSET_TYPE_LINE
        );

        rd = rrddim_add(st, "uptime", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st, rd, uptime_msec(uptime_filename));
    rrdset_done(st);
    return 0;
}
