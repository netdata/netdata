// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FILE_METADATA_H
#define NETDATA_FILE_METADATA_H

#include "libnetdata/libnetdata.h"
#include <stdint.h>
#include <time.h>

typedef struct {
    uint64_t size_bytes;      // File size in bytes
    time_t modified_time;     // Last modification time (Unix timestamp)
} OS_FILE_METADATA;

OS_FILE_METADATA os_get_file_metadata(const char *path);

#define OS_FILE_METADATA_OK(metadata) ((metadata).modified_time > 0 && (metadata).size_bytes > 0)

#endif //NETDATA_FILE_METADATA_H