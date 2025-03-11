// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MACHINE_ID_H
#define NETDATA_MACHINE_ID_H

#include "libnetdata/libnetdata.h"
#include "libnetdata/uuid/uuid.h"

/**
 * Special value returned when a reliable machine ID cannot be determined
 */
#define NO_MACHINE_ID (ND_UUID){ .parts = { .hig64 = 1, .low64 = 1 } }

/**
 * Get the machine ID
 *
 * Returns a UUID that uniquely identifies the machine.
 * On Linux, this is the systemd machine-id.
 * On other systems, it uses platform-specific values.
 *
 * The value is cached after first call.
 * Returns NO_MACHINE_ID when a reliable machine ID cannot be detected.
 *
 * @return ND_UUID The machine ID
 */
ND_UUID os_machine_id(void);

#endif // NETDATA_MACHINE_ID_H
