// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_SOURCES_H
#define NETDATA_WINDOWS_EVENTS_SOURCES_H

#include "libnetdata/libnetdata.h"

typedef enum {
    WEVTS_NONE                              = 0,
    WEVTS_ALL                               = (1 << 0),
    WEVTS_ADMIN                             = (1 << 1),
    WEVTS_OPERATIONAL                       = (1 << 2),
    WEVTS_ANALYTIC                          = (1 << 3),
    WEVTS_DEBUG                             = (1 << 4),
    WEVTS_WINDOWS                           = (1 << 5),
    WEVTS_ENABLED                           = (1 << 6),
    WEVTS_DISABLED                          = (1 << 7),
    WEVTS_FORWARDED                         = (1 << 8),
    WEVTS_CLASSIC                           = (1 << 9),
    WEVTS_BACKUP_MODE                       = (1 << 10),
    WEVTS_OVERWRITE_MODE                    = (1 << 11),
    WEVTS_STOP_WHEN_FULL_MODE               = (1 << 12),
    WEVTS_RETAIN_AND_BACKUP_MODE            = (1 << 13),
} WEVT_SOURCE_TYPE;

BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(WEVT_SOURCE_TYPE)

#define WEVT_SOURCE_ALL_NAME                        "All"
#define WEVT_SOURCE_ALL_ADMIN_NAME                  "All-Admin"
#define WEVT_SOURCE_ALL_OPERATIONAL_NAME            "All-Operational"
#define WEVT_SOURCE_ALL_ANALYTIC_NAME               "All-Analytic"
#define WEVT_SOURCE_ALL_DEBUG_NAME                  "All-Debug"
#define WEVT_SOURCE_ALL_WINDOWS_NAME                "All-Windows"
#define WEVT_SOURCE_ALL_ENABLED_NAME                "All-Enabled"
#define WEVT_SOURCE_ALL_DISABLED_NAME               "All-Disabled"
#define WEVT_SOURCE_ALL_FORWARDED_NAME              "All-Forwarded"
#define WEVT_SOURCE_ALL_CLASSIC_NAME                "All-Classic"
#define WEVT_SOURCE_ALL_BACKUP_MODE_NAME            "All-In-Backup-Mode"
#define WEVT_SOURCE_ALL_OVERWRITE_MODE_NAME         "All-In-Overwrite-Mode"
#define WEVT_SOURCE_ALL_STOP_WHEN_FULL_MODE_NAME    "All-In-StopWhenFull-Mode"
#define WEVT_SOURCE_ALL_RETAIN_AND_BACKUP_MODE_NAME "All-In-RetainAndBackup-Mode"

#define WEVT_SOURCE_ALL_OF_PROVIDER_PREFIX          "All-Of-"

typedef struct {
    const char *fullname;
    size_t fullname_len;

    const wchar_t *custom_query;

    STRING *source;
    STRING *provider;
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

void wevt_sources_init(void);
void wevt_sources_scan(void);
void buffer_json_wevt_versions(BUFFER *wb);

void wevt_sources_to_json_array(BUFFER *wb);
WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value);

int wevt_sources_dict_items_backward_compar(const void *a, const void *b);
int wevt_sources_dict_items_forward_compar(const void *a, const void *b);

#endif //NETDATA_WINDOWS_EVENTS_SOURCES_H
