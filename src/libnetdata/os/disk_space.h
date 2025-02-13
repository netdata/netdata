// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DISK_SPACE_H
#define NETDATA_DISK_SPACE_H

#include "libnetdata/libnetdata.h"

typedef struct {
    uint64_t total_bytes;   // Total disk size in bytes
    uint64_t free_bytes;    // Available disk space in bytes
    uint64_t total_inodes;  // Total number of inodes
    uint64_t free_inodes;   // Available inodes
} OS_SYSTEM_DISK_SPACE;

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path);

#endif //NETDATA_DISK_SPACE_H
