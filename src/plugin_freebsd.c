#include "common.h"

void *freebsd_main(void *ptr)
{
    (void)ptr;

    info("FREEBSD Plugin thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // disable (by default) various interface that are not needed
    /*
    config_get_boolean("plugin:proc:/proc/net/dev:lo", "enabled", 0);
    config_get_boolean("plugin:proc:/proc/net/dev:fireqos_monitor", "enabled", 0);
    */

    // when ZERO, attempt to do it
    int vdo_cpu_netdata             = !config_get_boolean("plugin:freebsd", "netdata server resources", 1);
    int vdo_freebsd_sysctl          = !config_get_boolean("plugin:freebsd", "sysctl", 1);

    // keep track of the time each module was called
    unsigned long long sutime_freebsd_sysctl = 0ULL;

    usec_t step = rrd_update_every * USEC_PER_SEC;
    for(;;) {
        usec_t now = now_realtime_usec();
        usec_t next = now - (now % step) + step;

        while(now < next) {
            sleep_usec(next - now);
            now = now_realtime_usec();
        }

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        if(!vdo_freebsd_sysctl) {
            debug(D_PROCNETDEV_LOOP, "FREEBSD: calling do_freebsd_sysctl().");
            now = now_realtime_usec();
            vdo_freebsd_sysctl = do_freebsd_sysctl(rrd_update_every, (sutime_freebsd_sysctl > 0)?now - sutime_freebsd_sysctl:0ULL);
            sutime_freebsd_sysctl = now;
        }
        if(unlikely(netdata_exit)) break;

        // END -- the job is done

        // --------------------------------------------------------------------

        if(!vdo_cpu_netdata) {
            global_statistics_charts();
            registry_statistics();
        }
    }

    info("FREEBSD thread exiting");

    pthread_exit(NULL);
    return NULL;
}

int getsysctl(const char *name, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}
