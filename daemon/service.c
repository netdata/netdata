// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

/* Run service jobs every X seconds */
#define SERVICE_HEARTBEAT 10

static void service_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    debug(D_SYSTEM, "Cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/*
 * The service thread.
 */
void *service_main(void *ptr)
{
    netdata_thread_cleanup_push(service_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * SERVICE_HEARTBEAT;

    debug(D_SYSTEM, "Service thread starts");

    while (!netdata_exit) {
        heartbeat_next(&hb, step);

        rrd_cleanup_obsolete_charts();

        rrd_wrlock();
        rrdhost_cleanup_orphan_hosts_nolock(localhost);
        rrd_unlock();

    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
