// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

static void exporting_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/**
 * Exporting engine main
 *
 * The main thread used to control the exporting engine.
 *
 * @param ptr a pointer to netdata_static_structure.
 *
 * @return It always returns NULL.
 */
void *exporting_main(void *ptr)
{
    netdata_thread_cleanup_push(exporting_main_cleanup, ptr);

    struct engine *engine = read_exporting_config();
    if (!engine) {
        info("EXPORTING: no exporting connectors configured");
        goto cleanup;
    }

    if (init_connectors(engine) != 0) {
        error("EXPORTING: cannot initialize exporting connectors");
        goto cleanup;
    }

    usec_t step_ut = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!netdata_exit) {
        heartbeat_next(&hb, step_ut);
        engine->now = now_realtime_sec();

        if (mark_scheduled_instances(engine)) {
            if (prepare_buffers(engine) != 0) {
                error("EXPORTING: cannot prepare data to send");
                break;
            }
        }

        if (notify_workers(engine) != 0) {
            error("EXPORTING: cannot communicate with exporting connector instance working threads");
            break;
        }

        if (send_internal_metrics(engine) != 0) {
            error("EXPORTING: cannot send metrics for the operation of exporting engine");
            break;
        }

#ifdef UNIT_TESTING
        break;
#endif
    }

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
