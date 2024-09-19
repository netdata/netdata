// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_SOURCES_H
#define NETDATA_WINDOWS_EVENTS_SOURCES_H

#include "libnetdata/libnetdata.h"

typedef enum {
    WEVTS_NONE               = 0,
    WEVTS_ALL                = (1 << 0),
    WEVTS_ADMIN              = (1 << 1),
    WEVTS_OPERATIONAL        = (1 << 2),
    WEVTS_ANALYTIC           = (1 << 3),
    WEVTS_DEBUG              = (1 << 4),
    WEVTS_DIAGNOSTIC         = (1 << 5),
    WEVTS_TRACING            = (1 << 6),
    WEVTS_PERFORMANCE        = (1 << 7),
    WEVTS_WINDOWS            = (1 << 8),
} WEVT_SOURCE_TYPE;

typedef struct {
    const char *fullname;
    size_t fullname_len;

    const wchar_t *custom_query;

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

extern DICTIONARY *wevt_sources;
extern DICTIONARY *used_hashes_registry;

#define WEVT_SOURCE_ALL_NAME                "All"
#define WEVT_SOURCE_ALL_ADMIN_NAME          "All-Admin"
#define WEVT_SOURCE_ALL_OPERATIONAL_NAME    "All-Operational"
#define WEVT_SOURCE_ALL_ANALYTIC_NAME       "All-Analytic"
#define WEVT_SOURCE_ALL_DEBUG_NAME          "All-Debug"
#define WEVT_SOURCE_ALL_DIAGNOSTIC_NAME     "All-Diagnostic"
#define WEVT_SOURCE_ALL_TRACING_NAME        "All-Tracing"
#define WEVT_SOURCE_ALL_PERFORMANCE_NAME    "All-Performance"
#define WEVT_SOURCE_ALL_WINDOWS_NAME        "All-Windows"

void wevt_sources_init(void);
void wevt_sources_scan(void);
void buffer_json_wevt_versions(BUFFER *wb);

void wevt_sources_to_json_array(BUFFER *wb);
WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value);

int wevt_sources_dict_items_backward_compar(const void *a, const void *b);
int wevt_sources_dict_items_forward_compar(const void *a, const void *b);

#endif //NETDATA_WINDOWS_EVENTS_SOURCES_H
