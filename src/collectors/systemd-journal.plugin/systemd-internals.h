// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
#define NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "provider/netdata_provider.h"

#include <linux/capability.h>
#include <syslog.h>

#define ND_SD_JOURNAL_FUNCTION_DESCRIPTION "View, search and analyze systemd journal entries."
#define ND_SD_JOURNAL_FUNCTION_NAME "systemd-journal"
#define ND_SD_JOURNAL_DEFAULT_TIMEOUT 60
#define ND_SD_JOURNAL_ENABLE_ESTIMATIONS_FILE_PERCENTAGE 0.01
#define ND_SD_JOURNAL_EXECUTE_WATCHER_PENDING_EVERY_MS 250
#define ND_SD_JOURNAL_ALL_FILES_SCAN_EVERY_USEC (5 * 60 * USEC_PER_SEC)

#define ND_SD_UNITS_FUNCTION_DESCRIPTION "Lists all systemd units (services, timers, mounts, etc.) with their current state and status."
#define ND_SD_UNITS_FUNCTION_NAME "systemd-list-units"
#define ND_SD_UNITS_DEFAULT_TIMEOUT 30

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
    ND_SD_JF_NONE = 0,
    ND_SD_JF_ALL = (1 << 0),
    ND_SD_JF_LOCAL_ALL = (1 << 1),
    ND_SD_JF_REMOTE_ALL = (1 << 2),
    ND_SD_JF_LOCAL_SYSTEM = (1 << 3),
    ND_SD_JF_LOCAL_USER = (1 << 4),
    ND_SD_JF_LOCAL_NAMESPACE = (1 << 5),
    ND_SD_JF_LOCAL_OTHER = (1 << 6),
} SD_JOURNAL_FILE_SOURCE_TYPE;

struct nd_journal_file {
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
    NsdId128 first_writer_id;
    NsdId128 last_writer_id;

    uint64_t messages_in_file;
};

#define ND_SD_JF_SOURCE_ALL_NAME "all"
#define ND_SD_JF_SOURCE_LOCAL_NAME "all-local-logs"
#define ND_SD_JF_SOURCE_LOCAL_SYSTEM_NAME "all-local-system-logs"
#define ND_SD_JF_SOURCE_LOCAL_USERS_NAME "all-local-user-logs"
#define ND_SD_JF_SOURCE_LOCAL_OTHER_NAME "all-uncategorized"
#define ND_SD_JF_SOURCE_NAMESPACES_NAME "all-local-namespaces"
#define ND_SD_JF_SOURCE_REMOTES_NAME "all-remote-systems"

#define ND_SD_JOURNAL_OPEN_FLAGS (0)

#define JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT (5 * USEC_PER_SEC)  // assume a 5-seconds latency
#define JOURNAL_VS_REALTIME_DELTA_MAX_UT (2 * 60 * USEC_PER_SEC) // up to 2-minutes latency

extern DICTIONARY *nd_journal_files_registry;
extern DICTIONARY *used_hashes_registry;
extern DICTIONARY *boot_ids_to_first_ut;

int nd_journal_file_dict_items_backward_compar(const void *a, const void *b);
int nd_journal_file_dict_items_forward_compar(const void *a, const void *b);
void buffer_json_journal_versions(BUFFER *wb);
void available_journal_file_sources_to_json_array(BUFFER *wb);
bool nd_journal_files_completed_once(void);
void nd_journal_files_registry_update(void);
void nd_journal_directory_scan_recursively(DICTIONARY *files, DICTIONARY *dirs, const char *dirname, int depth);

FACET_ROW_SEVERITY syslog_priority_to_facet_severity(FACETS *facets, FACET_ROW *row, void *data);

void nd_sd_journal_dynamic_row_id(
    FACETS *facets,
    BUFFER *json_array,
    FACET_ROW_KEY_VALUE *rkv,
    FACET_ROW *row,
    void *data);
void nd_sd_journal_transform_priority(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_syslog_facility(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_errno(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_boot_id(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_uid(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_gid(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_cap_effective(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);
void nd_sd_journal_transform_timestamp_usec(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);

usec_t nd_journal_file_update_annotation_boot_id(NsdJournal *j, struct nd_journal_file *njf, const char *boot_id);

#define MAX_JOURNAL_DIRECTORIES 100
struct journal_directory {
    STRING *path;
};
extern struct journal_directory journal_directories[MAX_JOURNAL_DIRECTORIES];

void nd_journal_init_files_and_directories(void);
void function_systemd_journal(
    const char *transaction,
    char *function,
    usec_t *stop_monotonic_ut,
    bool *cancelled,
    BUFFER *payload,
    HTTP_ACCESS access __maybe_unused,
    const char *source,
    void *data);
void nd_journal_file_update_header(const char *filename, struct nd_journal_file *njf);

void nd_sd_journal_annotations_init(void);
void nd_sd_journal_transform_message_id(FACETS *facets, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope, void *data);

void nd_journal_watcher_main(void *arg);
void nd_journal_watcher_restart(void);

static inline bool parse_journal_field(
    const char *data,
    size_t data_length,
    const char **key,
    size_t *key_length,
    const char **value,
    size_t *value_length)
{
    const char *k = data;
    const char *equal = strchr(k, '=');
    if (unlikely(!equal))
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

void nd_systemd_journal_dyncfg_init(struct functions_evloop_globals *wg);

bool is_journal_file(const char *filename, ssize_t len, const char **start_of_extension);

#endif //NETDATA_COLLECTORS_SYSTEMD_INTERNALS_H
