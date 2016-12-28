#include "common.h"
#include <sys/sysctl.h>

#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

// MacOS calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int getsysctl(const char *name, void *ptr, size_t len);

int do_macos_sysctl(int update_every, usec_t dt) {
    (void)dt;

    static int do_loadavg = -1, do_swap = -1;

    if (unlikely(do_loadavg == -1)) {
        do_loadavg              = config_get_boolean("plugin:macos:sysctl", "enable load average", 1);
        do_swap                 = config_get_boolean("plugin:macos:sysctl", "system swap", 1);
    }

    RRDSET *st;

    int system_pagesize = getpagesize(); // wouldn't it be better to get value directly from hw.pagesize?
    int i, n;
    int common_error = 0;
    size_t size;

    // NEEDED BY: do_loadavg
    static usec_t last_loadavg_usec = 0;
    struct loadavg sysload;

    // NEEDED BY: do_swap
    struct xsw_usage swap_usage;

    if (last_loadavg_usec <= dt) {
        if (likely(do_loadavg)) {
            if (unlikely(GETSYSCTL("vm.loadavg", sysload))) {
                do_loadavg = 0;
                error("DISABLED: system.load");
            } else {

                st = rrdset_find_bytype("system", "load");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "load", NULL, "load", NULL, "System Load Average", "load", 100, (update_every < MIN_LOADAVG_UPDATE_EVERY) ? MIN_LOADAVG_UPDATE_EVERY : update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "load1", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load5", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load15", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "load1", (collected_number) ((double)sysload.ldavg[0] / sysload.fscale * 1000));
                rrddim_set(st, "load5", (collected_number) ((double)sysload.ldavg[1] / sysload.fscale * 1000));
                rrddim_set(st, "load15", (collected_number) ((double)sysload.ldavg[2] / sysload.fscale * 1000));
                rrdset_done(st);
            }
        }

        last_loadavg_usec = st->update_every * USEC_PER_SEC;
    }
    else last_loadavg_usec -= dt;

    if (likely(do_swap)) {
        if (unlikely(GETSYSCTL("vm.swapusage", swap_usage))) {
            do_swap = 0;
            error("DISABLED: system.swap");
        } else {
            st = rrdset_find("system.swap");
            if (unlikely(!st)) {
                st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
                st->isdetail = 1;

                rrddim_add(st, "free",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
                rrddim_add(st, "used",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "free", swap_usage.xsu_avail);
            rrddim_set(st, "used", swap_usage.xsu_used);
            rrdset_done(st);
        }
    }

    return 0;
}

int getsysctl(const char *name, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("MACOS: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("MACOS: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}
