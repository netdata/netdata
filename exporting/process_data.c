// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Mark scheduled instances
 *
 * Any instance can have its own update interval. On every exporting engine update only those instances are picked,
 * which are scheduled for the update.
 *
 * @param engine an engine data structure
 * @return Returns 0 on success, 1 on failure
 */
int mark_scheduled_instances(struct engine *engine)
{
    (void)engine;

    return 0;
}

/**
 * Prepare buffers
 *
 * Walk through the Netdata database and fill buffers for every scheduled exporting connector instance according to
 * configured rules.
 *
 * @param engine an engine data structure
 * @return Returns 0 on success, 1 on failure
 */
int prepare_buffers(struct engine *engine)
{
    (void)engine;

    return 0;
}

/**
 * Notify workers
 *
 * Notify exporting connector instance working threads that data is ready to send.
 *
 * @param engine an engine data structure
 * @return Returns 0 on success, 1 on failure
 */
int notify_workers(struct engine *engine)
{
    (void)engine;

    return 0;
}
