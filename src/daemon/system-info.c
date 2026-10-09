// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static void system_info_cleanup(void *ptr) {
    struct netdata_static_thread *thread = CLEANUP_FUNCTION_GET_PTR(ptr);
    thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void system_info_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(system_info_cleanup) cleanup_ptr = ptr;
    worker_register("SYSTEM_INFO");
    worker_register_job_name(0, "detect");
    worker_register_job_name(1, "publish");

    const usec_t initial_delay = 5 * 60 * USEC_PER_SEC;
    const usec_t interval = 30 * 60 * USEC_PER_SEC;
    usec_t next = now_monotonic_usec() + initial_delay;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (service_running(SERVICE_SYSTEM_INFO)) {
        worker_is_idle();
        heartbeat_next(&hb);
        if (!service_running(SERVICE_SYSTEM_INFO))
            break;
        if (now_monotonic_usec() < next)
            continue;

        worker_is_busy(0);
        struct rrdhost_system_info *candidate = rrdhost_system_info_create();
        bool valid = rrdhost_system_info_detect_runtime(candidate);
        if (valid && service_running(SERVICE_SYSTEM_INFO)) {
            worker_is_busy(1);
            rrdhost_refresh_system_info(localhost, candidate);
        }
        rrdhost_system_info_free(candidate);
        next = now_monotonic_usec() + interval;
    }
}
