// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MACOS_LOGS_H
#define NETDATA_MACOS_LOGS_H

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"

#define MACOS_LOGS_FUNCTION_DESCRIPTION "View, search and analyze macOS unified logs."
#define MACOS_LOGS_FUNCTION_NAME "macos-logs"

#define MACOS_LOGS_WORKER_THREADS 2
#define MACOS_LOGS_DEFAULT_TIMEOUT 60
#define MACOS_LOGS_PROGRESS_EVERY_UT (250 * USEC_PER_MS)
#define MACOS_LOGS_PROGRESS_EVERY_ROWS 2000
#define MACOS_LOGS_DATA_ONLY_CHECK_EVERY_ROWS 1000
#define MACOS_LOGS_ANCHOR_DELTA_UT (10 * USEC_PER_SEC)
#define MACOS_LOGS_MAX_ROWS_SCANNED 1000000

#define MACOS_LOGS_FIELD_MESSAGE "MESSAGE"
#define MACOS_LOGS_FIELD_LEVEL "LEVEL"
#define MACOS_LOGS_FIELD_LEVEL_ID "LEVEL_ID"
#define MACOS_LOGS_FIELD_PROCESS "PROCESS"
#define MACOS_LOGS_FIELD_PID "PID"
#define MACOS_LOGS_FIELD_SENDER "SENDER"
#define MACOS_LOGS_FIELD_SUBSYSTEM "SUBSYSTEM"
#define MACOS_LOGS_FIELD_CATEGORY "CATEGORY"
#define MACOS_LOGS_FIELD_ENTRY_TYPE "ENTRY_TYPE"
#define MACOS_LOGS_FIELD_STORE_CATEGORY "STORE_CATEGORY"
#define MACOS_LOGS_FIELD_THREAD_ID "THREAD_ID"
#define MACOS_LOGS_FIELD_ACTIVITY_ID "ACTIVITY_ID"

typedef enum {
    MACOS_LOGS_QUERY_OK,
    MACOS_LOGS_QUERY_OPEN_FAILED,
    MACOS_LOGS_QUERY_ENUMERATOR_FAILED,
    MACOS_LOGS_QUERY_TIMED_OUT,
    MACOS_LOGS_QUERY_CANCELLED,
    MACOS_LOGS_QUERY_SCAN_LIMIT_REACHED,
} MACOS_LOGS_QUERY_STATUS;

typedef enum {
    MACOS_LOGS_SOURCE_NONE = 0,
    MACOS_LOGS_SOURCE_ALL = 1 << 0,
} MACOS_LOGS_SOURCE_TYPE;

struct lqs_extension {
    usec_t query_started_ut;
    usec_t query_finished_ut;
    usec_t progress_last_ut;

    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t rows_scanned_limit;
};

static inline MACOS_LOGS_SOURCE_TYPE macos_logs_source_type(const char *value) {
    if(!value || !*value)
        return MACOS_LOGS_SOURCE_NONE;

    if(strcasecmp(value, "all") == 0 ||
       strcasecmp(value, "system") == 0 ||
       strcasecmp(value, "macos-unified-log") == 0)
        return MACOS_LOGS_SOURCE_ALL;

    return MACOS_LOGS_SOURCE_NONE;
}

static inline void macos_logs_sources_to_json_array(BUFFER *wb) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", "all");
        buffer_json_member_add_string(wb, "name", "macOS unified log");
        buffer_json_member_add_string(wb, "pill", "native");
        buffer_json_member_add_string(wb, "info", "Native macOS unified log store queried through Apple's OSLog framework.");
        buffer_json_member_add_boolean(wb, "default_selected", true);
    }
    buffer_json_object_close(wb);
}

#define LQS_DEFAULT_SLICE_MODE 0
#define LQS_FUNCTION_NAME MACOS_LOGS_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION MACOS_LOGS_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING MACOS_LOGS_MAX_ROWS_SCANNED
#define LQS_SOURCE_TYPE MACOS_LOGS_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL MACOS_LOGS_SOURCE_ALL
#define LQS_SOURCE_TYPE_NONE MACOS_LOGS_SOURCE_NONE
#define LQS_PARAMETER_SOURCE_NAME "macOS Log Sources"
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) macos_logs_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) macos_logs_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

extern netdata_mutex_t stdout_mutex;
extern DICTIONARY *used_hashes_registry;

MACOS_LOGS_QUERY_STATUS macos_logs_query_oslog(LOGS_QUERY_STATUS *lqs);
void function_macos_logs(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                         BUFFER *payload, HTTP_ACCESS access, const char *source, void *data);

#endif // NETDATA_MACOS_LOGS_H
