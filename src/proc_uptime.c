#include "common.h"

static collected_number read_proc_uptime(void) {
    static procfile *ff = NULL;

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/uptime");

        ff = procfile_open(config_get("plugin:proc:/proc/uptime", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    if(unlikely(procfile_lines(ff) < 1)) {
        error("/proc/uptime has no lines.");
        return 1;
    }
    if(unlikely(procfile_linewords(ff, 0) < 1)) {
        error("/proc/uptime has less than 1 word in it.");
        return 1;
    }

    return (collected_number)(strtold(procfile_lineword(ff, 0, 0), NULL) * 1000.0);
}

int do_proc_uptime(int update_every, usec_t dt) {
    (void)dt;

    collected_number uptime;

#ifdef CLOCK_BOOTTIME_IS_AVAILABLE
    static int use_boottime = -1;

    if(unlikely(use_boottime == -1)) {
        long long delta = (long long)(now_boottime_usec() / 1000) - (long long)read_proc_uptime();
        if(delta < 0) delta = -delta;

        if(delta <= 1000) {
            info("Using now_boottime_usec() for uptime (dt is %lld ms)", delta);
            use_boottime = 1;
        }
        else {
            info("Using /proc/uptime for uptime (dt is %lld ms)", delta);
            use_boottime = 0;
        }
    }

    if(use_boottime)
        uptime = now_boottime_usec() / 1000;
    else
        uptime = read_proc_uptime();

#else
    uptime = read_proc_uptime();
#endif

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
