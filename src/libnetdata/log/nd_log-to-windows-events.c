// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS)

bool nd_log_wevents_init(void) {
    if(nd_log.wevents.initialized)
        return true;

    /*
     * TODO
     * 1. connect to windows events log
     * 2. check that our channel exists
     * 3. set nd_log.wevents.initialized = true;
     * 4. return true when everything is fine
     */

    return false;
}

bool nd_logger_wevents(struct log_field *fields, size_t fields_max) {
    if(!nd_log.wevents.initialized)
        return false;

    /*
     * TODO
     * 1. prepare the log
     * 2. send it to windows events
     * 3. return true when successful
     */

    return false;
}

#endif