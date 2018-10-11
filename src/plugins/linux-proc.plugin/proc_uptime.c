// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

static inline collected_number uptime_from_boottime(void) {
#ifdef CLOCK_BOOTTIME_IS_AVAILABLE
    return now_boottime_usec() / 1000;
#else
    error("uptime cannot be read from CLOCK_BOOTTIME on this system.");
    return 0;
#endif
}

static procfile *read_proc_uptime_ff = NULL;
static inline collected_number read_proc_uptime(void) {
    if(unlikely(!read_proc_uptime_ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/uptime");

        read_proc_uptime_ff = procfile_open(config_get("plugin:proc:/proc/uptime", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!read_proc_uptime_ff)) return 0;
    }

    read_proc_uptime_ff = procfile_readall(read_proc_uptime_ff);
    if(unlikely(!read_proc_uptime_ff)) return 0;

    if(unlikely(procfile_lines(read_proc_uptime_ff) < 1)) {
        error("/proc/uptime has no lines.");
        return 0;
    }
    if(unlikely(procfile_linewords(read_proc_uptime_ff, 0) < 1)) {
        error("/proc/uptime has less than 1 word in it.");
        return 0;
    }

    return (collected_number)(strtold(procfile_lineword(read_proc_uptime_ff, 0, 0), NULL) * 1000.0);
}

int do_proc_uptime(int update_every, usec_t dt) {
    (void)dt;

    static int use_boottime = -1;

    if(unlikely(use_boottime == -1)) {
        collected_number uptime_boottime = uptime_from_boottime();
        collected_number uptime_proc     = read_proc_uptime();

        long long delta = (long long)uptime_boottime - (long long)uptime_proc;
        if(delta < 0) delta = -delta;

        if(delta <= 1000 && uptime_boottime != 0) {
            procfile_close(read_proc_uptime_ff);
            info("Using now_boottime_usec() for uptime (dt is %lld ms)", delta);
            use_boottime = 1;
        }
        else if(uptime_proc != 0) {
            info("Using /proc/uptime for uptime (dt is %lld ms)", delta);
            use_boottime = 0;
        }
        else {
            error("Cannot find any way to read uptime on this system.");
            return 1;
        }
    }

    collected_number uptime;
    if(use_boottime)
        uptime = uptime_from_boottime();
    else
        uptime = read_proc_uptime();


    // --------------------------------------------------------------------

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
                , "proc"
                , "uptime"
                , 1000
                , update_every
                , RRDSET_TYPE_LINE
        );

        rd = rrddim_add(st, "uptime", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(st);

    rrddim_set_by_pointer(st, rd, uptime);

    rrdset_done(st);

    return 0;
}
