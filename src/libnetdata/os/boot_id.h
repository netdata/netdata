// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_BOOT_ID_H
#define NETDATA_OS_BOOT_ID_H

#include "libnetdata/common.h"
#include "libnetdata/uuid/uuid.h"

/**
 * Get system boot ID
 *
 * Returns a UUID that remains constant during system uptime.
 * On Linux, this is the systemd boot_id.
 * On other systems, this uses the system boot time to generate a unique ID.
 *
 * The value is cached after first call.
 * Returns UUID_ZERO on error.
 *
 * @return ND_UUID The boot ID
 */
ND_UUID os_boot_id(void);

bool os_boot_ids_match(ND_UUID a, ND_UUID b);

#endif
