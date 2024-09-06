// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_SOURCES_H
#define NETDATA_WINDOWS_EVENTS_SOURCES_H

#include "windows-events.h"

typedef enum {
    WEVTS_NONE               = 0,
    WEVTS_ALL                = (1 << 0),
} WEVT_SOURCE_TYPE;

typedef struct {
    const char *fullname;
    size_t fullname_len;

    STRING *source;
    WEVT_SOURCE_TYPE source_type;
    usec_t msg_first_ut;
    usec_t msg_last_ut;
    size_t size;

    usec_t last_scan_monotonic_ut;

    uint64_t msg_first_id;
    uint64_t msg_last_id;
    uint64_t entries;
} LOGS_QUERY_SOURCE;

#define WEVT_SOURCE_ALL_NAME "all"

void wevt_sources_init(void);
void wevt_sources_scan(void);
void buffer_json_wevt_versions(BUFFER *wb);

void wevt_sources_to_json_array(BUFFER *wb);
WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value);

#endif //NETDATA_WINDOWS_EVENTS_SOURCES_H
