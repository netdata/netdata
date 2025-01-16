// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HOSTNAME_H
#define NETDATA_HOSTNAME_H

#include "libnetdata/libnetdata.h"

/**
 * Get system hostname in UTF-8 encoding
 *
 * @param dst Buffer to store the hostname
 * @param dst_size Size of the destination buffer
 * @param filesystem_root Optional root directory (for Unix-like systems, when running in a container)
 * @return true on success, false on failure
 *
 * The function guarantees to return a UTF-8 encoded hostname.
 * On Windows, filesystem_root is ignored.
 */
 bool os_hostname(char *dst, size_t dst_size, const char *filesystem_root);

#endif //NETDATA_HOSTNAME_H
