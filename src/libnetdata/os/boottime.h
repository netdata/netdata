// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BOOTTIME_H
#define NETDATA_BOOTTIME_H

#include "libnetdata/common.h"

/**
 * Get system boot time
 *
 * Returns the absolute wallclock timestamp (Unix epoch) of when the system was last booted.
 * The value is cached after first successful call.
 * Returns 0 on error.
 *
 * @return time_t The boot timestamp, 0 on error
 */
time_t os_boottime(void);

#endif //NETDATA_BOOTTIME_H
