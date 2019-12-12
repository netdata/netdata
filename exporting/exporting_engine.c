// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

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
    (void)ptr;

    struct engine *engine = read_exporting_config();
    if (!engine) {
        info("EXPORTING: no exporting connectors configured");
        return NULL;
    }

    if (init_connectors(engine) != 0) {
        error("EXPORTING: cannot initialize exporting connectors");
        return NULL;
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
                return NULL;
            }
        }

        if (notify_workers(engine) != 0) {
            error("EXPORTING: cannot communicate with exporting connector instance working threads");
            return NULL;
        }

        if (send_internal_metrics(engine) != 0) {
            error("EXPORTING: cannot send metrics for the operation of exporting engine");
            return NULL;
        }

#ifdef UNIT_TESTING
        break;
#endif
    }

    return NULL;
}
