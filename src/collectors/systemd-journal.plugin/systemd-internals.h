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
#define SYSTEMD_JOURNAL_ENABLE_ESTIMATIONS_FILE_PERCENTAGE 0.01
#define SYSTEMD_JOURNAL_EXECUTE_WATCHER_PENDING_EVERY_MS 250
#define SYSTEMD_JOURNAL_ALL_FILES_SCAN_EVERY_USEC (5 * 60 * USEC_PER_SEC)

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
    size_t size;
    bool logged_failure;
    bool logged_journalctl_failure;
    usec_t max_journal_vs_realtime_delta_ut;

    usec_t last_scan_monotonic_ut;
    usec_t last_scan_header_vs_last_modified_ut;

    uint64_t first_seqnum;
    uint64_t last_seqnum;
    sd_id128_t first_writer_id;
    sd_id128_t last_writer_id;

    uint64_t messages_in_file;
};

#define SDJF_SOURCE_ALL_NAME "all"
#define SDJF_SOURCE_LOCAL_NAME "all-local-logs"
#define SDJF_SOURCE_LOCAL_SYSTEM_NAME "all-local-system-logs"
#define SDJF_SOURCE_LOCAL_USERS_NAME "all-local-user-logs"
#define SDJF_SOURCE_LOCAL_OTHER_NAME "all-uncategorized"
#define SDJF_SOURCE_NAMESPACES_NAME "all-local-namespaces"
#define SDJF_SOURCE_REMOTES_NAME "all-remote-systems"

#define ND_SD_JOURNAL_OPEN_FLAGS (0)

#define JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT (5 * USEC_PER_SEC) // assume a 5-seconds latency
#define JOURNAL_VS_REALTIME_DELTA_MAX_UT (2 * 60 * USEC_PER_SEC) // up to 2-minutes latency

extern DICTIONARY *journal_files_registry;
extern DICTIONARY *used_hashes_registry;
extern DICTIONARY *boot_ids_to_first_ut;

int journal_file_dict_items_backward_compar(const void *a, const void *b);
int journal_file_dict_items_forward_compar(const void *a, const void *b);
void buffer_json_journal_versions(BUFFER *wb);
void available_journal_file_sources_to_json_array(BUFFER *wb);
bool journal_files_completed_once(void);
void journal_files_registry_update(void);
void journal_directory_scan_recursively(DICTIONARY *files, DICTIONARY *dirs, const char *dirname, int depth);

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

usec_t journal_file_update_annotation_boot_id(sd_journal *j, struct journal_file *jf, const char *boot_id);

#define MAX_JOURNAL_DIRECTORIES 100
struct journal_directory {
    STRING *path;
};
extern struct journal_directory journal_directories[MAX_JOURNAL_DIRECTORIES];

void journal_init_files_and_directories(void);
void function_systemd_journal(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *payload, HTTP_ACCESS access __maybe_unused, const char *source, void *data);
void journal_file_update_header(const char *filename, struct journal_file *jf);

void netdata_systemd_journal_annotations_init(void);
void netdata_systemd_journal_transform_message_id(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);

void *journal_watcher_main(void *arg);
void journal_watcher_restart(void);

#ifdef ENABLE_SYSTEMD_DBUS
void function_systemd_units(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *payload, HTTP_ACCESS access __maybe_unused, const char *source, void *data);
#endif

static inline bool parse_journal_field(const char *data, size_t data_length, const char **key, size_t *key_length, const char **value, size_t *value_length) {
    const char *k = data;
    const char *equal = strchr(k, '=');
    if(unlikely(!equal))
        return false;

    size_t kl = equal - k;

    const char *v = ++equal;
    size_t vl = data_length - kl - 1;

    *key = k;
    *key_length = kl;
    *value = v;
    *value_length = vl;

    return true;
}

void systemd_journal_dyncfg_init(struct functions_evloop_globals *wg);

bool is_journal_file(const char *filename, ssize_t len, const char **start_of_extension);

#endif //NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
