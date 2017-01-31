#include "common.h"

void *freebsd_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("FREEBSD Plugin thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // when ZERO, attempt to do it
    int vdo_cpu_netdata             = !config_get_boolean("plugin:freebsd", "netdata server resources", 1);
    int vdo_freebsd_sysctl          = !config_get_boolean("plugin:freebsd", "sysctl", 1);

    // keep track of the time each module was called
    unsigned long long sutime_freebsd_sysctl = 0ULL;

    usec_t step = rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;;) {
        usec_t hb_dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        // BEGIN -- the job to be done

        if(!vdo_freebsd_sysctl) {
            debug(D_PROCNETDEV_LOOP, "FREEBSD: calling do_freebsd_sysctl().");
            vdo_freebsd_sysctl = do_freebsd_sysctl(rrd_update_every, hb_dt);
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

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
