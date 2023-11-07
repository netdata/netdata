// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
#define NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"

#include <linux/capability.h>
#include <systemd/sd-journal.h>
#include <syslog.h>

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         60

#define SYSTEMD_UNITS_FUNCTION_DESCRIPTION      "View the status of systemd units"
#define SYSTEMD_UNITS_FUNCTION_NAME              "systemd-list-units"
#define SYSTEMD_UNITS_DEFAULT_TIMEOUT            30

extern __thread size_t fstat_thread_calls;
extern __thread size_t fstat_thread_cached_responses;
void fstat_cache_enable_on_thread(void);
void fstat_cache_disable_on_thread(void);

extern netdata_mutex_t stdout_mutex;


typedef enum {
    ND_SD_JOURNAL_NO_FILE_MATCHED,
    ND_SD_JOURNAL_FAILED_TO_OPEN,
    ND_SD_JOURNAL_FAILED_TO_SEEK,
    ND_SD_JOURNAL_TIMED_OUT,
    ND_SD_JOURNAL_OK,
    ND_SD_JOURNAL_NOT_MODIFIED,
    ND_SD_JOURNAL_CANCELLED,
} ND_SD_JOURNAL_STATUS;

typedef enum {
    SDJF_NONE               = 0,
    SDJF_ALL                = (1 << 0),
    SDJF_LOCAL_ALL          = (1 << 1),
    SDJF_REMOTE_ALL         = (1 << 2),
    SDJF_LOCAL_SYSTEM       = (1 << 3),
    SDJF_LOCAL_USER         = (1 << 4),
    SDJF_LOCAL_NAMESPACE    = (1 << 5),
    SDJF_LOCAL_OTHER        = (1 << 6),
} SD_JOURNAL_FILE_SOURCE_TYPE;

struct journal_file {
    const char *filename;
    size_t filename_len;
    STRING *source;
    SD_JOURNAL_FILE_SOURCE_TYPE source_type;
    usec_t file_last_modified_ut;
    usec_t msg_first_ut;
    usec_t msg_last_ut;
    usec_t last_scan_ut;
    size_t size;
    bool logged_failure;
    usec_t max_journal_vs_realtime_delta_ut;
};

#define SDJF_SOURCE_ALL_NAME "all"
#define SDJF_SOURCE_LOCAL_NAME "all-local-logs"
#define SDJF_SOURCE_LOCAL_SYSTEM_NAME "all-local-system-logs"
#define SDJF_SOURCE_LOCAL_USERS_NAME "all-local-user-logs"
#define SDJF_SOURCE_LOCAL_OTHER_NAME "all-uncategorized"
#define SDJF_SOURCE_NAMESPACES_NAME "all-local-namespaces"
#define SDJF_SOURCE_REMOTES_NAME "all-remote-systems"

#define ND_SD_JOURNAL_OPEN_FLAGS (0)

#define JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT (5 * USEC_PER_SEC) // assume always 5 seconds latency
#define JOURNAL_VS_REALTIME_DELTA_MAX_UT (2 * 60 * USEC_PER_SEC) // up to 2 minutes latency

extern DICTIONARY *journal_files_registry;
extern DICTIONARY *used_hashes_registry;
extern DICTIONARY *function_query_status_dict;
extern DICTIONARY *boot_ids_to_first_ut;

int journal_file_dict_items_backward_compar(const void *a, const void *b);
int journal_file_dict_items_forward_compar(const void *a, const void *b);
void buffer_json_journal_versions(BUFFER *wb);
void available_journal_file_sources_to_json_array(BUFFER *wb);

FACET_ROW_SEVERITY syslog_priority_to_facet_severity(FACETS *facets, FACET_ROW *row, void *data);

void netdata_systemd_journal_dynamic_row_id(FACETS *facets, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data);
void netdata_systemd_journal_transform_priority(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_syslog_facility(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_errno(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_boot_id(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_uid(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_gid(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_cap_effective(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void netdata_systemd_journal_transform_timestamp_usec(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);

void journal_init_files_and_directories(void);
void journal_init_query_status(void);
void function_systemd_journal(const char *transaction, char *function, int timeout, bool *cancelled);
void journal_files_registry_update(void);

#ifdef ENABLE_SYSTEMD_DBUS
void function_systemd_units(const char *transaction, char *function, int timeout, bool *cancelled);
#endif

#endif //NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
