// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Send internal metrics
 *
 * Send performance metrics for the operation of exporting engine itself to the Netdata database.
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int send_internal_metrics(struct engine *engine)
{
    (void)engine;

    return 0;
}
