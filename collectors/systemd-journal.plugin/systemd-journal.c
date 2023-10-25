// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <linux/capability.h>
#include <systemd/sd-journal.h>
#include <syslog.h>

/*
 * TODO
 *
 * _UDEV_DEVLINK is frequently set more than once per field - support multi-value faces
 *
 */


// ----------------------------------------------------------------------------
// fstat64 overloading to speed up libsystemd
// https://github.com/systemd/systemd/pull/29261

#define ND_SD_JOURNAL_OPEN_FLAGS (0)

#include <dlfcn.h>
#include <sys/stat.h>

#define FSTAT_CACHE_MAX 1024
struct fdstat64_cache_entry {
    bool enabled;
    bool updated;
    int err_no;
    struct stat64 stat;
    int ret;
    size_t cached_count;
    size_t session;
};

struct fdstat64_cache_entry fstat64_cache[FSTAT_CACHE_MAX] = {0 };
static __thread size_t fstat_thread_calls = 0;
static __thread size_t fstat_thread_cached_responses = 0;
static __thread bool enable_thread_fstat = false;
static __thread size_t fstat_caching_thread_session = 0;
static size_t fstat_caching_global_session = 0;

static void fstat_cache_enable_on_thread(void) {
    fstat_caching_thread_session = __atomic_add_fetch(&fstat_caching_global_session, 1, __ATOMIC_ACQUIRE);
    enable_thread_fstat = true;
}

static void fstat_cache_disable_on_thread(void) {
    fstat_caching_thread_session = __atomic_add_fetch(&fstat_caching_global_session, 1, __ATOMIC_RELEASE);
    enable_thread_fstat = false;
}

int fstat64(int fd, struct stat64 *buf) {
    static int (*real_fstat)(int, struct stat64 *) = NULL;
    if (!real_fstat)
        real_fstat = dlsym(RTLD_NEXT, "fstat64");

    fstat_thread_calls++;

    if(fd >= 0 && fd < FSTAT_CACHE_MAX) {
        if(enable_thread_fstat && fstat64_cache[fd].session != fstat_caching_thread_session) {
            fstat64_cache[fd].session = fstat_caching_thread_session;
            fstat64_cache[fd].enabled = true;
            fstat64_cache[fd].updated = false;
        }

        if(fstat64_cache[fd].enabled && fstat64_cache[fd].updated && fstat64_cache[fd].session == fstat_caching_thread_session) {
            fstat_thread_cached_responses++;
            errno = fstat64_cache[fd].err_no;
            *buf = fstat64_cache[fd].stat;
            fstat64_cache[fd].cached_count++;
            return fstat64_cache[fd].ret;
        }
    }

    int ret = real_fstat(fd, buf);

    if(fd >= 0 && fd < FSTAT_CACHE_MAX && fstat64_cache[fd].enabled && fstat64_cache[fd].session == fstat_caching_thread_session) {
        fstat64_cache[fd].ret = ret;
        fstat64_cache[fd].updated = true;
        fstat64_cache[fd].err_no = errno;
        fstat64_cache[fd].stat = *buf;
    }

    return ret;
}

// ----------------------------------------------------------------------------

#define FACET_MAX_VALUE_LENGTH                  8192
#define SYSTEMD_JOURNAL_MAX_SOURCE_LEN          64

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         60
#define SYSTEMD_JOURNAL_MAX_PARAMS              100
#define SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION  (1 * 3600)
#define SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY 200
#define SYSTEMD_JOURNAL_WORKER_THREADS          5

#define JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT (5 * USEC_PER_SEC) // assume always 5 seconds latency
#define JOURNAL_VS_REALTIME_DELTA_MAX_UT (2 * 60 * USEC_PER_SEC) // up to 2 minutes latency

#define JOURNAL_PARAMETER_HELP                  "help"
#define JOURNAL_PARAMETER_AFTER                 "after"
#define JOURNAL_PARAMETER_BEFORE                "before"
#define JOURNAL_PARAMETER_ANCHOR                "anchor"
#define JOURNAL_PARAMETER_LAST                  "last"
#define JOURNAL_PARAMETER_QUERY                 "query"
#define JOURNAL_PARAMETER_FACETS                "facets"
#define JOURNAL_PARAMETER_HISTOGRAM             "histogram"
#define JOURNAL_PARAMETER_DIRECTION             "direction"
#define JOURNAL_PARAMETER_IF_MODIFIED_SINCE     "if_modified_since"
#define JOURNAL_PARAMETER_DATA_ONLY             "data_only"
#define JOURNAL_PARAMETER_SOURCE                "source"
#define JOURNAL_PARAMETER_INFO                  "info"
#define JOURNAL_PARAMETER_ID                    "id"
#define JOURNAL_PARAMETER_PROGRESS              "progress"
#define JOURNAL_PARAMETER_SLICE                 "slice"
#define JOURNAL_PARAMETER_DELTA                 "delta"
#define JOURNAL_PARAMETER_TAIL                  "tail"

#define JOURNAL_KEY_ND_JOURNAL_FILE             "ND_JOURNAL_FILE"
#define JOURNAL_KEY_ND_JOURNAL_PROCESS          "ND_JOURNAL_PROCESS"

#define JOURNAL_DEFAULT_SLICE_MODE              true
#define JOURNAL_DEFAULT_DIRECTION               FACETS_ANCHOR_DIRECTION_BACKWARD

#define SYSTEMD_ALWAYS_VISIBLE_KEYS             NULL

#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS       \
    "*MESSAGE*"                                 \
    "|*_RAW"                                    \
    "|*_USEC"                                   \
    "|*_NSEC"                                   \
    "|*TIMESTAMP*"                              \
    "|*_ID"                                     \
    "|*_ID_*"                                   \
    "|__*"                                      \
    ""

#define SYSTEMD_KEYS_INCLUDED_IN_FACETS         \
                                                \
    /* --- USER JOURNAL FIELDS --- */           \
                                                \
    /* "|MESSAGE" */                            \
    /* "|MESSAGE_ID" */                         \
    "|PRIORITY"                                 \
    "|CODE_FILE"                                \
    /* "|CODE_LINE" */                          \
    "|CODE_FUNC"                                \
    "|ERRNO"                                    \
    /* "|INVOCATION_ID" */                      \
    /* "|USER_INVOCATION_ID" */                 \
    "|SYSLOG_FACILITY"                          \
    "|SYSLOG_IDENTIFIER"                        \
    /* "|SYSLOG_PID" */                         \
    /* "|SYSLOG_TIMESTAMP" */                   \
    /* "|SYSLOG_RAW" */                         \
    /* "!DOCUMENTATION" */                      \
    /* "|TID" */                                \
    "|UNIT"                                     \
    "|USER_UNIT"                                \
    "|UNIT_RESULT" /* undocumented */           \
                                                \
                                                \
    /* --- TRUSTED JOURNAL FIELDS --- */        \
                                                \
    /* "|_PID" */                               \
    "|_UID"                                     \
    "|_GID"                                     \
    "|_COMM"                                    \
    "|_EXE"                                     \
    /* "|_CMDLINE" */                           \
    "|_CAP_EFFECTIVE"                           \
    /* "|_AUDIT_SESSION" */                     \
    "|_AUDIT_LOGINUID"                          \
    "|_SYSTEMD_CGROUP"                          \
    "|_SYSTEMD_SLICE"                           \
    "|_SYSTEMD_UNIT"                            \
    "|_SYSTEMD_USER_UNIT"                       \
    "|_SYSTEMD_USER_SLICE"                      \
    "|_SYSTEMD_SESSION"                         \
    "|_SYSTEMD_OWNER_UID"                       \
    "|_SELINUX_CONTEXT"                         \
    /* "|_SOURCE_REALTIME_TIMESTAMP" */         \
    "|_BOOT_ID"                                 \
    "|_MACHINE_ID"                              \
    /* "|_SYSTEMD_INVOCATION_ID" */             \
    "|_HOSTNAME"                                \
    "|_TRANSPORT"                               \
    "|_STREAM_ID"                               \
    /* "|LINE_BREAK" */                         \
    "|_NAMESPACE"                               \
    "|_RUNTIME_SCOPE"                           \
                                                \
                                                \
    /* --- KERNEL JOURNAL FIELDS --- */         \
                                                \
    /* "|_KERNEL_DEVICE" */                     \
    "|_KERNEL_SUBSYSTEM"                        \
    /* "|_UDEV_SYSNAME" */                      \
    "|_UDEV_DEVNODE"                            \
    /* "|_UDEV_DEVLINK" */                      \
                                                \
                                                \
    /* --- LOGGING ON BEHALF --- */             \
                                                \
    "|OBJECT_UID"                               \
    "|OBJECT_GID"                               \
    "|OBJECT_COMM"                              \
    "|OBJECT_EXE"                               \
    /* "|OBJECT_CMDLINE" */                     \
    /* "|OBJECT_AUDIT_SESSION" */               \
    "|OBJECT_AUDIT_LOGINUID"                    \
    "|OBJECT_SYSTEMD_CGROUP"                    \
    "|OBJECT_SYSTEMD_SESSION"                   \
    "|OBJECT_SYSTEMD_OWNER_UID"                 \
    "|OBJECT_SYSTEMD_UNIT"                      \
    "|OBJECT_SYSTEMD_USER_UNIT"                 \
                                                \
                                                \
    /* --- CORE DUMPS --- */                    \
                                                \
    "|COREDUMP_COMM"                            \
    "|COREDUMP_UNIT"                            \
    "|COREDUMP_USER_UNIT"                       \
    "|COREDUMP_SIGNAL_NAME"                     \
    "|COREDUMP_CGROUP"                          \
                                                \
                                                \
    /* --- DOCKER --- */                        \
                                                \
    "|CONTAINER_ID"                             \
    /* "|CONTAINER_ID_FULL" */                  \
    "|CONTAINER_NAME"                           \
    "|CONTAINER_TAG"                            \
    "|IMAGE_NAME" /* undocumented */            \
    /* "|CONTAINER_PARTIAL_MESSAGE" */          \
                                                \
    ""

static netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

// ----------------------------------------------------------------------------

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

typedef struct function_query_status {
    bool *cancelled; // a pointer to the cancelling boolean
    usec_t stop_monotonic_ut;

    usec_t started_monotonic_ut;

    // request
    SD_JOURNAL_FILE_SOURCE_TYPE source_type;
    SIMPLE_PATTERN *sources;
    usec_t after_ut;
    usec_t before_ut;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
    } anchor;

    FACETS_ANCHOR_DIRECTION direction;
    size_t entries;
    usec_t if_modified_since;
    bool delta;
    bool tail;
    bool data_only;
    bool slice;
    size_t filters;
    usec_t last_modified;
    const char *query;
    const char *histogram;

    // per file progress info
    size_t cached_count;

    // progress statistics
    usec_t matches_setup_ut;
    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t files_matched;
    size_t file_working;
} FUNCTION_QUERY_STATUS;

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

static void log_fqs(FUNCTION_QUERY_STATUS *fqs, const char *msg) {
    netdata_log_error("ERROR: %s, on query "
                      "timeframe [%"PRIu64" - %"PRIu64"], "
                      "anchor [%"PRIu64" - %"PRIu64"], "
                      "if_modified_since %"PRIu64", "
                      "data_only:%s, delta:%s, tail:%s, direction:%s"
                      , msg
                      , fqs->after_ut, fqs->before_ut
                      , fqs->anchor.start_ut, fqs->anchor.stop_ut
                      , fqs->if_modified_since
                      , fqs->data_only ? "true" : "false"
                      , fqs->delta ? "true" : "false"
                      , fqs->tail ? "tail" : "false"
                      , fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
}

static inline bool netdata_systemd_journal_seek_to(sd_journal *j, usec_t timestamp) {
    if(sd_journal_seek_realtime_usec(j, timestamp) < 0) {
        netdata_log_error("SYSTEMD-JOURNAL: Failed to seek to %" PRIu64, timestamp);
        if(sd_journal_seek_tail(j) < 0) {
            netdata_log_error("SYSTEMD-JOURNAL: Failed to seek to journal's tail");
            return false;
        }
    }

    return true;
}

#define JD_SOURCE_REALTIME_TIMESTAMP "_SOURCE_REALTIME_TIMESTAMP"

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

static inline size_t netdata_systemd_journal_process_row(sd_journal *j, FACETS *facets, struct journal_file *jf, usec_t *msg_ut) {
    const void *data;
    size_t length, bytes = 0;

    facets_add_key_value_length(facets, JOURNAL_KEY_ND_JOURNAL_FILE, sizeof(JOURNAL_KEY_ND_JOURNAL_FILE) - 1, jf->filename, jf->filename_len);

    SD_JOURNAL_FOREACH_DATA(j, data, length) {
        const char *key, *value;
        size_t key_length, value_length;

        if(!parse_journal_field(data, length, &key, &key_length, &value, &value_length))
            continue;

#ifdef NETDATA_INTERNAL_CHECKS
        usec_t origin_journal_ut = *msg_ut;
#endif
        if(unlikely(key_length == sizeof(JD_SOURCE_REALTIME_TIMESTAMP) - 1 &&
            memcmp(key, JD_SOURCE_REALTIME_TIMESTAMP, sizeof(JD_SOURCE_REALTIME_TIMESTAMP) - 1) == 0)) {
            usec_t ut = str2ull(value, NULL);
            if(ut && ut < *msg_ut) {
                usec_t delta = *msg_ut - ut;
                *msg_ut = ut;

                if(delta > JOURNAL_VS_REALTIME_DELTA_MAX_UT)
                    delta = JOURNAL_VS_REALTIME_DELTA_MAX_UT;

                // update max_journal_vs_realtime_delta_ut if the delta increased
                usec_t expected = jf->max_journal_vs_realtime_delta_ut;
                do {
                    if(delta <= expected)
                        break;
                } while(!__atomic_compare_exchange_n(&jf->max_journal_vs_realtime_delta_ut, &expected, delta, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

                internal_error(delta > expected,
                               "increased max_journal_vs_realtime_delta_ut from %"PRIu64" to %"PRIu64", "
                               "journal %"PRIu64", actual %"PRIu64" (delta %"PRIu64")"
                               , expected, delta, origin_journal_ut, *msg_ut, origin_journal_ut - (*msg_ut));
            }
        }

        bytes += length;
        facets_add_key_value_length(facets, key, key_length, value, value_length <= FACET_MAX_VALUE_LENGTH ? value_length : FACET_MAX_VALUE_LENGTH);
    }

    return bytes;
}

#define FUNCTION_PROGRESS_UPDATE_ROWS(rows_read, rows) __atomic_fetch_add(&(rows_read), rows, __ATOMIC_RELAXED)
#define FUNCTION_PROGRESS_UPDATE_BYTES(bytes_read, bytes) __atomic_fetch_add(&(bytes_read), bytes, __ATOMIC_RELAXED)
#define FUNCTION_PROGRESS_EVERY_ROWS (1ULL << 13)
#define FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS (1ULL << 7)

static inline ND_SD_JOURNAL_STATUS check_stop(const bool *cancelled, const usec_t *stop_monotonic_ut) {
    if(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)) {
        internal_error(true, "Function has been cancelled");
        return ND_SD_JOURNAL_CANCELLED;
    }

    if(now_monotonic_usec() > __atomic_load_n(stop_monotonic_ut, __ATOMIC_RELAXED)) {
        internal_error(true, "Function timed out");
        return ND_SD_JOURNAL_TIMED_OUT;
    }

    return ND_SD_JOURNAL_OK;
}

ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_backward(
        sd_journal *j, BUFFER *wb __maybe_unused, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = ((fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->before_ut) + anchor_delta;
    usec_t stop_ut = (fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->after_ut;
    bool stop_when_full = (fqs->data_only && !fqs->anchor.stop_ut);

    if(!netdata_systemd_journal_seek_to(j, start_ut))
        return ND_SD_JOURNAL_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t earliest_msg_ut = 0;
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_OK;

    facets_rows_begin(facets);
    while (status == ND_SD_JOURNAL_OK && sd_journal_previous(j) > 0) {
        usec_t msg_ut = 0;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0 || !msg_ut) {
            errors_no_timestamp++;
            continue;
        }

        if(unlikely(msg_ut > earliest_msg_ut))
            earliest_msg_ut = msg_ut;

        if (unlikely(msg_ut > start_ut))
            continue;

        if (unlikely(msg_ut < stop_ut))
            break;

        bytes += netdata_systemd_journal_process_row(j, facets, jf, &msg_ut);

        // make sure each line gets a unique timestamp
        if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
            msg_ut = --last_usec_from;
        else
            last_usec_from = last_usec_to = msg_ut;

        if(facets_row_finished(facets, msg_ut))
            rows_useful++;

        row_counter++;
        if(unlikely((row_counter % FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS) == 0 &&
            stop_when_full &&
            facets_rows(facets) >= fqs->entries)) {
            // stop the data only query
            usec_t oldest = facets_row_oldest_ut(facets);
            if(oldest && msg_ut < (oldest - anchor_delta))
                break;
        }

        if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
            FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
            last_row_counter = row_counter;

            FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);
            last_bytes = bytes;

            status = check_stop(fqs->cancelled, &fqs->stop_monotonic_ut);
        }
    }

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);

    fqs->rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("SYSTEMD-JOURNAL: %zu lines did not have timestamps", errors_no_timestamp);

    if(earliest_msg_ut > fqs->last_modified)
        fqs->last_modified = earliest_msg_ut;

    return status;
}

ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_forward(
        sd_journal *j, BUFFER *wb __maybe_unused, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = (fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->after_ut;
    usec_t stop_ut = ((fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->before_ut) + anchor_delta;
    bool stop_when_full = (fqs->data_only && !fqs->anchor.stop_ut);

    if(!netdata_systemd_journal_seek_to(j, start_ut))
        return ND_SD_JOURNAL_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t earliest_msg_ut = 0;
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_OK;

    facets_rows_begin(facets);
    while (status == ND_SD_JOURNAL_OK && sd_journal_next(j) > 0) {
        usec_t msg_ut = 0;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0 || !msg_ut) {
            errors_no_timestamp++;
            continue;
        }

        if(likely(msg_ut > earliest_msg_ut))
            earliest_msg_ut = msg_ut;

        if (unlikely(msg_ut < start_ut))
            continue;

        if (unlikely(msg_ut > stop_ut))
            break;

        bytes += netdata_systemd_journal_process_row(j, facets, jf, &msg_ut);

        // make sure each line gets a unique timestamp
        if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
            msg_ut = ++last_usec_to;
        else
            last_usec_from = last_usec_to = msg_ut;

        if(facets_row_finished(facets, msg_ut))
            rows_useful++;

        row_counter++;
        if(unlikely((row_counter % FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS) == 0 &&
            stop_when_full &&
            facets_rows(facets) >= fqs->entries)) {
            // stop the data only query
            usec_t newest = facets_row_newest_ut(facets);
            if(newest && msg_ut > (newest + anchor_delta))
                break;
        }

        if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
            FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
            last_row_counter = row_counter;

            FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);
            last_bytes = bytes;

            status = check_stop(fqs->cancelled, &fqs->stop_monotonic_ut);
        }
    }

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);

    fqs->rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("SYSTEMD-JOURNAL: %zu lines did not have timestamps", errors_no_timestamp);

    if(earliest_msg_ut > fqs->last_modified)
        fqs->last_modified = earliest_msg_ut;

    return status;
}

bool netdata_systemd_journal_check_if_modified_since(sd_journal *j, usec_t seek_to, usec_t last_modified) {
    // return true, if data have been modified since the timestamp

    if(!last_modified || !seek_to)
        return false;

    if(!netdata_systemd_journal_seek_to(j, seek_to))
        return false;

    usec_t first_msg_ut = 0;
    while (sd_journal_previous(j) > 0) {
        usec_t msg_ut;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0)
            continue;

        first_msg_ut = msg_ut;
        break;
    }

    return first_msg_ut != last_modified;
}

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
static bool netdata_systemd_filtering_by_journal(sd_journal *j, FACETS *facets, FUNCTION_QUERY_STATUS *fqs) {
    const char *field = NULL;
    const void *data = NULL;
    size_t data_length;
    size_t added_keys = 0;
    size_t failures = 0;
    size_t filters_added = 0;

    SD_JOURNAL_FOREACH_FIELD(j, field) {
        bool interesting;

        if(fqs->data_only)
            interesting = facets_key_name_is_filter(facets, field);
        else
            interesting = facets_key_name_is_facet(facets, field);

        if(interesting) {
            if(sd_journal_query_unique(j, field) >= 0) {
                bool added_this_key = false;
                size_t added_values = 0;

                SD_JOURNAL_FOREACH_UNIQUE(j, data, data_length) {
                    const char *key, *value;
                    size_t key_length, value_length;

                    if(!parse_journal_field(data, data_length, &key, &key_length, &value, &value_length))
                        continue;

                    facets_add_possible_value_name_to_key(facets, key, key_length, value, value_length);

                    if(!facets_key_name_value_length_is_selected(facets, key, key_length, value, value_length))
                        continue;

                    if(added_keys && !added_this_key) {
                        if(sd_journal_add_conjunction(j) < 0)
                            failures++;

                        added_this_key = true;
                        added_keys++;
                    }
                    else if(added_values)
                        if(sd_journal_add_disjunction(j) < 0)
                            failures++;

                    if(sd_journal_add_match(j, data, data_length) < 0)
                        failures++;

                    added_values++;
                    filters_added++;
                }
            }
        }
    }

    if(failures) {
        log_fqs(fqs, "failed to setup journal filter, will run the full query.");
        sd_journal_flush_matches(j);
        return true;
    }

    return filters_added ? true : false;
}
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

static ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_one_file(
        const char *filename, BUFFER *wb, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    sd_journal *j = NULL;
    errno = 0;

    fstat_cache_enable_on_thread();

    const char *paths[2] = {
            [0] = filename,
            [1] = NULL,
    };

    if(sd_journal_open_files(&j, paths, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
        netdata_log_error("JOURNAL: cannot open file '%s' for query", filename);
        fstat_cache_disable_on_thread();
        return ND_SD_JOURNAL_FAILED_TO_OPEN;
    }

    ND_SD_JOURNAL_STATUS status;
    bool matches_filters = true;

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    if(fqs->slice) {
        usec_t started = now_monotonic_usec();

        matches_filters = netdata_systemd_filtering_by_journal(j, facets, fqs) || !fqs->filters;
        usec_t ended = now_monotonic_usec();

        fqs->matches_setup_ut += (ended - started);
    }
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    if(matches_filters) {
        if(fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD)
            status = netdata_systemd_journal_query_forward(j, wb, facets, jf, fqs);
        else
            status = netdata_systemd_journal_query_backward(j, wb, facets, jf, fqs);
    }
    else
        status = ND_SD_JOURNAL_NO_FILE_MATCHED;

    sd_journal_close(j);
    fstat_cache_disable_on_thread();

    return status;
}

// ----------------------------------------------------------------------------
// journal files registry

#define VAR_LOG_JOURNAL_MAX_DEPTH 10
#define MAX_JOURNAL_DIRECTORIES 100

struct journal_directory {
    char *path;
    bool logged_failure;
};

static struct journal_directory journal_directories[MAX_JOURNAL_DIRECTORIES] = { 0 };
static DICTIONARY *journal_files_registry = NULL;
static DICTIONARY *used_hashes_registry = NULL;

static usec_t systemd_journal_session = 0;

static void buffer_json_journal_versions(BUFFER *wb) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources",
                                      systemd_journal_session + dictionary_version(journal_files_registry));
    }
    buffer_json_object_close(wb);
}

static void journal_file_update_msg_ut(const char *filename, struct journal_file *jf) {
    fstat_cache_enable_on_thread();

    const char *files[2] = {
            [0] = filename,
            [1] = NULL,
    };

    sd_journal *j = NULL;
    if(sd_journal_open_files(&j, files, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
        netdata_log_error("JOURNAL: cannot open file '%s' to update msg_ut", filename);
        fstat_cache_disable_on_thread();

        if(!jf->logged_failure) {
            netdata_log_error("cannot open journal file '%s', using file timestamps to understand time-frame.", filename);
            jf->logged_failure = true;
        }

        jf->msg_first_ut = 0;
        jf->msg_last_ut = jf->file_last_modified_ut;
        return;
    }

    usec_t first_ut = 0, last_ut = 0;

    if(sd_journal_seek_head(j) < 0 || sd_journal_next(j) < 0 || sd_journal_get_realtime_usec(j, &first_ut) < 0 || !first_ut) {
        internal_error(true, "cannot find the timestamp of the first message in '%s'", filename);
        first_ut = 0;
    }

    if(sd_journal_seek_tail(j) < 0 || sd_journal_previous(j) < 0 || sd_journal_get_realtime_usec(j, &last_ut) < 0 || !last_ut) {
        internal_error(true, "cannot find the timestamp of the last message in '%s'", filename);
        last_ut = jf->file_last_modified_ut;
    }

    sd_journal_close(j);
    fstat_cache_disable_on_thread();

    if(first_ut > last_ut) {
        internal_error(true, "timestamps are flipped in file '%s'", filename);
        usec_t t = first_ut;
        first_ut = last_ut;
        last_ut = t;
    }

    jf->msg_first_ut = first_ut;
    jf->msg_last_ut = last_ut;
}

static STRING *string_strdupz_source(const char *s, const char *e, size_t max_len, const char *prefix) {
    char buf[max_len];
    size_t len;
    char *dst = buf;

    if(prefix) {
        len = strlen(prefix);
        memcpy(buf, prefix, len);
        dst = &buf[len];
        max_len -= len;
    }

    len = e - s;
    if(len >= max_len)
        len = max_len - 1;
    memcpy(dst, s, len);
    dst[len] = '\0';
    buf[max_len - 1] = '\0';

    for(size_t i = 0; buf[i] ;i++)
        if(!isalnum(buf[i]) && buf[i] != '-' && buf[i] != '.' && buf[i] != ':')
            buf[i] = '_';

    return string_strdupz(buf);
}

static void files_registry_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    struct journal_file *jf = value;
    jf->filename = dictionary_acquired_item_name(item);
    jf->filename_len = strlen(jf->filename);
    jf->source_type = SDJF_ALL;

    // based on the filename
    // decide the source to show to the user
    const char *s = strrchr(jf->filename, '/');
    if(s) {
        if(strstr(jf->filename, "/remote/")) {
            jf->source_type |= SDJF_REMOTE_ALL;

            if(strncmp(s, "/remote-", 8) == 0) {
                s = &s[8]; // skip "/remote-"

                char *e = strchr(s, '@');
                if(!e)
                    e = strstr(s, ".journal");

                if(e) {
                    const char *d = s;
                    for(; d < e && (isdigit(*d) || *d == '.' || *d == ':') ; d++) ;
                    if(d == e) {
                        // a valid IP address
                        char ip[e - s + 1];
                        memcpy(ip, s, e - s);
                        ip[e - s] = '\0';
                        char buf[SYSTEMD_JOURNAL_MAX_SOURCE_LEN];
                        if(ip_to_hostname(ip, buf, sizeof(buf)))
                            jf->source = string_strdupz_source(buf, &buf[strlen(buf)], SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                        else {
                            internal_error(true, "Cannot find the hostname for IP '%s'", ip);
                            jf->source = string_strdupz_source(s, e, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                        }
                    }
                    else
                        jf->source = string_strdupz_source(s, e, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "remote-");
                }
            }
        }
        else {
            jf->source_type |= SDJF_LOCAL_ALL;

            const char *t = s - 1;
            while(t >= jf->filename && *t != '.' && *t != '/')
                t--;

            if(t >= jf->filename && *t == '.') {
                jf->source_type |= SDJF_LOCAL_NAMESPACE;
                jf->source = string_strdupz_source(t + 1, s, SYSTEMD_JOURNAL_MAX_SOURCE_LEN, "namespace-");
            }
            else if(strncmp(s, "/system", 7) == 0)
                jf->source_type |= SDJF_LOCAL_SYSTEM;

            else if(strncmp(s, "/user", 5) == 0)
                jf->source_type |= SDJF_LOCAL_USER;

            else
                jf->source_type |= SDJF_LOCAL_OTHER;
        }
    }
    else
        jf->source_type |= SDJF_LOCAL_ALL | SDJF_LOCAL_OTHER;

    journal_file_update_msg_ut(jf->filename, jf);

    internal_error(true,
                   "found journal file '%s', type %d, source '%s', "
                   "file modified: %"PRIu64", "
                   "msg {first: %"PRIu64", last: %"PRIu64"}",
                   jf->filename, jf->source_type, jf->source ? string2str(jf->source) : "<unset>",
                   jf->file_last_modified_ut,
                   jf->msg_first_ut, jf->msg_last_ut);
}

static bool files_registry_conflict_cb(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data __maybe_unused) {
    struct journal_file *jf = old_value;
    struct journal_file *njf = new_value;

    if(njf->last_scan_ut > jf->last_scan_ut)
        jf->last_scan_ut = njf->last_scan_ut;

    if(njf->file_last_modified_ut > jf->file_last_modified_ut) {
        jf->file_last_modified_ut = njf->file_last_modified_ut;
        jf->size = njf->size;

        const char *filename = dictionary_acquired_item_name(item);
        journal_file_update_msg_ut(filename, jf);

//        internal_error(true,
//                "updated journal file '%s', type %d, "
//                "file modified: %"PRIu64", "
//                                        "msg {first: %"PRIu64", last: %"PRIu64"}",
//                filename, jf->source_type,
//                jf->file_last_modified_ut,
//                jf->msg_first_ut, jf->msg_last_ut);
    }

    return false;
}

#define SDJF_SOURCE_ALL_NAME "all"
#define SDJF_SOURCE_LOCAL_NAME "all-local-logs"
#define SDJF_SOURCE_LOCAL_SYSTEM_NAME "all-local-system-logs"
#define SDJF_SOURCE_LOCAL_USERS_NAME "all-local-user-logs"
#define SDJF_SOURCE_LOCAL_OTHER_NAME "all-uncategorized"
#define SDJF_SOURCE_NAMESPACES_NAME "all-local-namespaces"
#define SDJF_SOURCE_REMOTES_NAME "all-remote-systems"

struct journal_file_source {
    usec_t first_ut;
    usec_t last_ut;
    size_t count;
    uint64_t size;
};

static void human_readable_size_ib(uint64_t size, char *dst, size_t dst_len) {
    if(size > 1024ULL * 1024 * 1024 * 1024)
        snprintfz(dst, dst_len, "%0.2f TiB", (double)size / 1024.0 / 1024.0 / 1024.0 / 1024.0);
    else if(size > 1024ULL * 1024 * 1024)
        snprintfz(dst, dst_len, "%0.2f GiB", (double)size / 1024.0 / 1024.0 / 1024.0);
    else if(size > 1024ULL * 1024)
        snprintfz(dst, dst_len, "%0.2f MiB", (double)size / 1024.0 / 1024.0);
    else if(size > 1024ULL)
        snprintfz(dst, dst_len, "%0.2f KiB", (double)size / 1024.0);
    else
        snprintfz(dst, dst_len, "%"PRIu64" B", size);
}

#define print_duration(dst, dst_len, pos, remaining, duration, one, many, printed) do { \
    if((remaining) > (duration)) {                                                      \
        uint64_t _count = (remaining) / (duration);                                     \
        uint64_t _rem = (remaining) - (_count * (duration));                            \
        (pos) += snprintfz(&(dst)[pos], (dst_len) - (pos), "%s%s%"PRIu64" %s", (printed) ? ", " : "", _rem ? "" : "and ", _count, _count > 1 ? (many) : (one));  \
        (remaining) = _rem;                                                             \
        (printed) = true;                                                               \
    } \
} while(0)

static void human_readable_duration_s(time_t duration_s, char *dst, size_t dst_len) {
    if(duration_s < 0)
        duration_s = -duration_s;

    size_t pos = 0;
    dst[0] = 0 ;

    bool printed = false;
    print_duration(dst, dst_len, pos, duration_s, 86400 * 365, "year", "years", printed);
    print_duration(dst, dst_len, pos, duration_s, 86400 * 30, "month", "months", printed);
    print_duration(dst, dst_len, pos, duration_s, 86400 * 1, "day", "days", printed);
    print_duration(dst, dst_len, pos, duration_s, 3600 * 1, "hour", "hours", printed);
    print_duration(dst, dst_len, pos, duration_s, 60 * 1, "min", "mins", printed);
    print_duration(dst, dst_len, pos, duration_s, 1, "sec", "secs", printed);
}

static int journal_file_to_json_array_cb(const DICTIONARY_ITEM *item, void *entry, void *data) {
    struct journal_file_source *jfs = entry;
    BUFFER *wb = data;

    const char *name = dictionary_acquired_item_name(item);

    buffer_json_add_array_item_object(wb);
    {
        char size_for_humans[100];
        human_readable_size_ib(jfs->size, size_for_humans, sizeof(size_for_humans));

        char duration_for_humans[1024];
        human_readable_duration_s((time_t)((jfs->last_ut - jfs->first_ut) / USEC_PER_SEC),
                                  duration_for_humans, sizeof(duration_for_humans));

        char info[1024];
        snprintfz(info, sizeof(info), "%zu files, with a total size of %s, covering %s",
                  jfs->count, size_for_humans, duration_for_humans);

        buffer_json_member_add_string(wb, "id", name);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_string(wb, "pill", size_for_humans);
        buffer_json_member_add_string(wb, "info", info);
    }
    buffer_json_object_close(wb); // options object

    return 1;
}

static bool journal_file_merge_sizes(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value , void *data __maybe_unused) {
    struct journal_file_source *jfs = old_value, *njfs = new_value;
    jfs->count += njfs->count;
    jfs->size += njfs->size;

    if(njfs->first_ut && njfs->first_ut < jfs->first_ut)
        jfs->first_ut = njfs->first_ut;

    if(njfs->last_ut && njfs->last_ut > jfs->last_ut)
        jfs->last_ut = njfs->last_ut;

    return false;
}

static void available_journal_file_sources_to_json_array(BUFFER *wb) {
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_NAME_LINK_DONT_CLONE|DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_conflict_callback(dict, journal_file_merge_sizes, NULL);

    struct journal_file_source t = { 0 };

    struct journal_file *jf;
    dfe_start_read(journal_files_registry, jf) {
        t.first_ut = jf->msg_first_ut;
        t.last_ut = jf->msg_last_ut;
        t.count = 1;
        t.size = jf->size;

        dictionary_set(dict, SDJF_SOURCE_ALL_NAME, &t, sizeof(t));

        if(jf->source_type & SDJF_LOCAL_ALL)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_SYSTEM)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_SYSTEM_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_USER)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_USERS_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_OTHER)
            dictionary_set(dict, SDJF_SOURCE_LOCAL_OTHER_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_LOCAL_NAMESPACE)
            dictionary_set(dict, SDJF_SOURCE_NAMESPACES_NAME, &t, sizeof(t));
        if(jf->source_type & SDJF_REMOTE_ALL)
            dictionary_set(dict, SDJF_SOURCE_REMOTES_NAME, &t, sizeof(t));
        if(jf->source)
            dictionary_set(dict, string2str(jf->source), &t, sizeof(t));
    }
    dfe_done(jf);

    dictionary_sorted_walkthrough_read(dict, journal_file_to_json_array_cb, wb);

    dictionary_destroy(dict);
}

static void files_registry_delete_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    struct journal_file *jf = value; (void)jf;
    const char *filename = dictionary_acquired_item_name(item); (void)filename;

    string_freez(jf->source);
    internal_error(true, "removed journal file '%s'", filename);
}

void journal_directory_scan(const char *dirname, int depth, usec_t last_scan_ut) {
    static const char *ext = ".journal";
    static const size_t ext_len = sizeof(".journal") - 1;

    if (depth > VAR_LOG_JOURNAL_MAX_DEPTH)
        return;

    DIR *dir;
    struct dirent *entry;
    struct stat info;
    char absolute_path[FILENAME_MAX];

    // Open the directory.
    if ((dir = opendir(dirname)) == NULL) {
        if(errno != ENOENT && errno != ENOTDIR)
            netdata_log_error("Cannot opendir() '%s'", dirname);
        return;
    }

    // Read each entry in the directory.
    while ((entry = readdir(dir)) != NULL) {
        snprintfz(absolute_path, sizeof(absolute_path), "%s/%s", dirname, entry->d_name);
        if (stat(absolute_path, &info) != 0) {
            netdata_log_error("Failed to stat() '%s", absolute_path);
            continue;
        }

        if (S_ISDIR(info.st_mode)) {
            // If entry is a directory, call traverse recursively.
            if (strcmp(entry->d_name, ".") != 0 && strcmp(entry->d_name, "..") != 0)
                journal_directory_scan(absolute_path, depth + 1, last_scan_ut);

        }
        else if (S_ISREG(info.st_mode)) {
            // If entry is a regular file, check if it ends with .journal.
            char *filename = entry->d_name;
            size_t len = strlen(filename);

            if (len > ext_len && strcmp(filename + len - ext_len, ext) == 0) {
                struct journal_file t = {
                        .file_last_modified_ut = info.st_mtim.tv_sec * USEC_PER_SEC + info.st_mtim.tv_nsec / NSEC_PER_USEC,
                        .last_scan_ut = last_scan_ut,
                        .size = info.st_size,
                        .max_journal_vs_realtime_delta_ut = JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT,
                };
                dictionary_set(journal_files_registry, absolute_path, &t, sizeof(t));
            }
        }
    }

    closedir(dir);
}

static void journal_files_registry_update() {
    usec_t scan_ut = now_monotonic_usec();

    for(unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES ;i++) {
        if(!journal_directories[i].path)
            break;

        journal_directory_scan(journal_directories[i].path, 0, scan_ut);
    }

    struct journal_file *jf;
    dfe_start_write(journal_files_registry, jf) {
        if(jf->last_scan_ut < scan_ut)
            dictionary_del(journal_files_registry, jf_dfe.name);
    }
    dfe_done(jf);
}

// ----------------------------------------------------------------------------

static bool jf_is_mine(struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    if((fqs->source_type == SDJF_NONE && !fqs->sources) || (jf->source_type & fqs->source_type) ||
        (fqs->sources && simple_pattern_matches(fqs->sources, string2str(jf->source)))) {

        usec_t anchor_delta = JOURNAL_VS_REALTIME_DELTA_MAX_UT;
        usec_t first_ut = jf->msg_first_ut;
        usec_t last_ut = jf->msg_last_ut + anchor_delta;

        if(last_ut >= fqs->after_ut && first_ut <= fqs->before_ut)
            return true;
    }

    return false;
}

static int journal_file_dict_items_backward_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM **ad = (const DICTIONARY_ITEM **)a, **bd = (const DICTIONARY_ITEM **)b;
    struct journal_file *jfa = dictionary_acquired_item_value(*ad);
    struct journal_file *jfb = dictionary_acquired_item_value(*bd);

    if(jfa->msg_last_ut < jfb->msg_last_ut)
        return 1;

    if(jfa->msg_last_ut > jfb->msg_last_ut)
        return -1;

    if(jfa->msg_first_ut < jfb->msg_first_ut)
        return 1;

    if(jfa->msg_first_ut > jfb->msg_first_ut)
        return -1;

    return 0;
}

static int journal_file_dict_items_forward_compar(const void *a, const void *b) {
    return -journal_file_dict_items_backward_compar(a, b);
}

static int netdata_systemd_journal_query(BUFFER *wb, FACETS *facets, FUNCTION_QUERY_STATUS *fqs) {
    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_NO_FILE_MATCHED;
    struct journal_file *jf;

    fqs->files_matched = 0;
    fqs->file_working = 0;
    fqs->rows_useful = 0;
    fqs->rows_read = 0;
    fqs->bytes_read = 0;

    size_t files_used = 0;
    size_t files_max = dictionary_entries(journal_files_registry);
    const DICTIONARY_ITEM *file_items[files_max];

    // count the files
    bool files_are_newer = false;
    dfe_start_read(journal_files_registry, jf) {
        if(!jf_is_mine(jf, fqs))
            continue;

        file_items[files_used++] = dictionary_acquired_item_dup(journal_files_registry, jf_dfe.item);

        if(jf->msg_last_ut > fqs->if_modified_since)
            files_are_newer = true;
    }
    dfe_done(jf);

    fqs->files_matched = files_used;

    if(fqs->if_modified_since && !files_are_newer) {
        buffer_flush(wb);
        return HTTP_RESP_NOT_MODIFIED;
    }

    // sort the files, so that they are optimal for facets
    if(files_used >= 2) {
        if (fqs->direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  journal_file_dict_items_backward_compar);
        else
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  journal_file_dict_items_forward_compar);
    }

    bool partial = false;
    usec_t started_ut;
    usec_t ended_ut = now_monotonic_usec();

    buffer_json_member_add_array(wb, "_journal_files");
    for(size_t f = 0; f < files_used ;f++) {
        const char *filename = dictionary_acquired_item_name(file_items[f]);
        jf = dictionary_acquired_item_value(file_items[f]);

        if(!jf_is_mine(jf, fqs))
            continue;

        fqs->file_working++;
        fqs->cached_count = 0;

        size_t fs_calls = fstat_thread_calls;
        size_t fs_cached = fstat_thread_cached_responses;
        size_t rows_useful = fqs->rows_useful;
        size_t rows_read = fqs->rows_read;
        size_t bytes_read = fqs->bytes_read;
        size_t matches_setup_ut = fqs->matches_setup_ut;

        ND_SD_JOURNAL_STATUS tmp_status = netdata_systemd_journal_query_one_file(filename, wb, facets, jf, fqs);

        rows_useful = fqs->rows_useful - rows_useful;
        rows_read = fqs->rows_read - rows_read;
        bytes_read = fqs->bytes_read - bytes_read;
        matches_setup_ut = fqs->matches_setup_ut - matches_setup_ut;
        fs_calls = fstat_thread_calls - fs_calls;
        fs_cached = fstat_thread_cached_responses - fs_cached;

        started_ut = ended_ut;
        ended_ut = now_monotonic_usec();
        usec_t duration_ut = ended_ut - started_ut;

        buffer_json_add_array_item_object(wb); // journal file
        {
            // information about the file
            buffer_json_member_add_string(wb, "_filename", filename);
            buffer_json_member_add_uint64(wb, "_source_type", jf->source_type);
            buffer_json_member_add_string(wb, "_source", string2str(jf->source));
            buffer_json_member_add_uint64(wb, "_last_modified_ut", jf->file_last_modified_ut);
            buffer_json_member_add_uint64(wb, "_msg_first_ut", jf->msg_first_ut);
            buffer_json_member_add_uint64(wb, "_msg_last_ut", jf->msg_last_ut);
            buffer_json_member_add_uint64(wb, "_journal_vs_realtime_delta_ut", jf->max_journal_vs_realtime_delta_ut);

            // information about the current use of the file
            buffer_json_member_add_uint64(wb, "duration_ut", ended_ut - started_ut);
            buffer_json_member_add_uint64(wb, "rows_read", rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", rows_useful);
            buffer_json_member_add_double(wb, "rows_per_second", (double) rows_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "bytes_read", bytes_read);
            buffer_json_member_add_double(wb, "bytes_per_second", (double) bytes_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "duration_matches_ut", matches_setup_ut);
            buffer_json_member_add_uint64(wb, "fstat_query_calls", fs_calls);
            buffer_json_member_add_uint64(wb, "fstat_query_cached_responses", fs_cached);
        }
        buffer_json_object_close(wb); // journal file

        bool stop = false;
        switch(tmp_status) {
            case ND_SD_JOURNAL_OK:
            case ND_SD_JOURNAL_NO_FILE_MATCHED:
                status = (status == ND_SD_JOURNAL_OK) ? ND_SD_JOURNAL_OK : tmp_status;
                break;

            case ND_SD_JOURNAL_FAILED_TO_OPEN:
            case ND_SD_JOURNAL_FAILED_TO_SEEK:
                partial = true;
                if(status == ND_SD_JOURNAL_NO_FILE_MATCHED)
                    status = tmp_status;
                break;

            case ND_SD_JOURNAL_CANCELLED:
            case ND_SD_JOURNAL_TIMED_OUT:
                partial = true;
                stop = true;
                status = tmp_status;
                break;

            case ND_SD_JOURNAL_NOT_MODIFIED:
                internal_fatal(true, "this should never be returned here");
                break;
        }

        if(stop)
            break;
    }
    buffer_json_array_close(wb); // _journal_files

    // release the files
    for(size_t f = 0; f < files_used ;f++)
        dictionary_acquired_item_release(journal_files_registry, file_items[f]);

    switch (status) {
        case ND_SD_JOURNAL_OK:
            if(fqs->if_modified_since && !fqs->rows_useful) {
                buffer_flush(wb);
                return HTTP_RESP_NOT_MODIFIED;
            }
            break;

        case ND_SD_JOURNAL_TIMED_OUT:
        case ND_SD_JOURNAL_NO_FILE_MATCHED:
            break;

        case ND_SD_JOURNAL_CANCELLED:
            buffer_flush(wb);
            return HTTP_RESP_CLIENT_CLOSED_REQUEST;

        case ND_SD_JOURNAL_NOT_MODIFIED:
            buffer_flush(wb);
            return HTTP_RESP_NOT_MODIFIED;

        default:
        case ND_SD_JOURNAL_FAILED_TO_OPEN:
        case ND_SD_JOURNAL_FAILED_TO_SEEK:
            buffer_flush(wb);
            return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    if(!fqs->data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
    }

    if(!fqs->data_only || fqs->tail)
        buffer_json_member_add_uint64(wb, "last_modified", fqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + (fqs->data_only ? 3600 : 0));

    buffer_json_member_add_object(wb, "_fstat_caching");
    {
        buffer_json_member_add_uint64(wb, "calls", fstat_thread_calls);
        buffer_json_member_add_uint64(wb, "cached", fstat_thread_cached_responses);
    }
    buffer_json_object_close(wb); // _fstat_caching
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static void netdata_systemd_journal_function_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following parameters are supported:\n"
            "\n"
            "   "JOURNAL_PARAMETER_HELP"\n"
            "      Shows this help message.\n"
            "\n"
            "   "JOURNAL_PARAMETER_INFO"\n"
            "      Request initial configuration information about the plugin.\n"
            "      The key entity returned is the required_params array, which includes\n"
            "      all the available systemd journal sources.\n"
            "      When `"JOURNAL_PARAMETER_INFO"` is requested, all other parameters are ignored.\n"
            "\n"
            "   "JOURNAL_PARAMETER_ID":STRING\n"
            "      Caller supplied unique ID of the request.\n"
            "      This can be used later to request a progress report of the query.\n"
            "      Optional, but if omitted no `"JOURNAL_PARAMETER_PROGRESS"` can be requested.\n"
            "\n"
            "   "JOURNAL_PARAMETER_PROGRESS"\n"
            "      Request a progress report (the `id` of a running query is required).\n"
            "      When `"JOURNAL_PARAMETER_PROGRESS"` is requested, only parameter `"JOURNAL_PARAMETER_ID"` is used.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DATA_ONLY":true or "JOURNAL_PARAMETER_DATA_ONLY":false\n"
            "      Quickly respond with data requested, without generating a\n"
            "      `histogram`, `facets` counters and `items`.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DELTA":true or "JOURNAL_PARAMETER_DELTA":false\n"
            "      When doing data only queries, include deltas for histogram, facets and items.\n"
            "\n"
            "   "JOURNAL_PARAMETER_TAIL":true or "JOURNAL_PARAMETER_TAIL":false\n"
            "      When doing data only queries, respond with the newest messages,\n"
            "      and up to the anchor, but calculate deltas (if requested) for\n"
            "      the duration [anchor - before].\n"
            "\n"
            "   "JOURNAL_PARAMETER_SLICE":true or "JOURNAL_PARAMETER_SLICE":false\n"
            "      When it is turned on, the plugin is executing filtering via libsystemd,\n"
            "      utilizing all the available indexes of the journal files.\n"
            "      When it is off, only the time constraint is handled by libsystemd and\n"
            "      all filtering is done by the plugin.\n"
            "      The default is: %s\n"
            "\n"
            "   "JOURNAL_PARAMETER_SOURCE":SOURCE\n"
            "      Query only the specified journal sources.\n"
            "      Do an `"JOURNAL_PARAMETER_INFO"` query to find the sources.\n"
            "\n"
            "   "JOURNAL_PARAMETER_BEFORE":TIMESTAMP_IN_SECONDS\n"
            "      Absolute or relative (to now) timestamp in seconds, to start the query.\n"
            "      The query is always executed from the most recent to the oldest log entry.\n"
            "      If not given the default is: now.\n"
            "\n"
            "   "JOURNAL_PARAMETER_AFTER":TIMESTAMP_IN_SECONDS\n"
            "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n"
            "      If not given, the default is %d.\n"
            "\n"
            "   "JOURNAL_PARAMETER_LAST":ITEMS\n"
            "      The number of items to return.\n"
            "      The default is %d.\n"
            "\n"
            "   "JOURNAL_PARAMETER_ANCHOR":TIMESTAMP_IN_MICROSECONDS\n"
            "      Return items relative to this timestamp.\n"
            "      The exact items to be returned depend on the query `"JOURNAL_PARAMETER_DIRECTION"`.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DIRECTION":forward or "JOURNAL_PARAMETER_DIRECTION":backward\n"
            "      When set to `backward` (default) the items returned are the newest before the\n"
            "      `"JOURNAL_PARAMETER_ANCHOR"`, (or `"JOURNAL_PARAMETER_BEFORE"` if `"JOURNAL_PARAMETER_ANCHOR"` is not set)\n"
            "      When set to `forward` the items returned are the oldest after the\n"
            "      `"JOURNAL_PARAMETER_ANCHOR"`, (or `"JOURNAL_PARAMETER_AFTER"` if `"JOURNAL_PARAMETER_ANCHOR"` is not set)\n"
            "      The default is: %s\n"
            "\n"
            "   "JOURNAL_PARAMETER_QUERY":SIMPLE_PATTERN\n"
            "      Do a full text search to find the log entries matching the pattern given.\n"
            "      The plugin is searching for matches on all fields of the database.\n"
            "\n"
            "   "JOURNAL_PARAMETER_IF_MODIFIED_SINCE":TIMESTAMP_IN_MICROSECONDS\n"
            "      Each successful response, includes a `last_modified` field.\n"
            "      By providing the timestamp to the `"JOURNAL_PARAMETER_IF_MODIFIED_SINCE"` parameter,\n"
            "      the plugin will return 200 with a successful response, or 304 if the source has not\n"
            "      been modified since that timestamp.\n"
            "\n"
            "   "JOURNAL_PARAMETER_HISTOGRAM":facet_id\n"
            "      Use the given `facet_id` for the histogram.\n"
            "      This parameter is ignored in `"JOURNAL_PARAMETER_DATA_ONLY"` mode.\n"
            "\n"
            "   "JOURNAL_PARAMETER_FACETS":facet_id1,facet_id2,facet_id3,...\n"
            "      Add the given facets to the list of fields for which analysis is required.\n"
            "      The plugin will offer both a histogram and facet value counters for its values.\n"
            "      This parameter is ignored in `"JOURNAL_PARAMETER_DATA_ONLY"` mode.\n"
            "\n"
            "   facet_id:value_id1,value_id2,value_id3,...\n"
            "      Apply filters to the query, based on the facet IDs returned.\n"
            "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n"
            "\n"
            , program_name
            , SYSTEMD_JOURNAL_FUNCTION_NAME
            , SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION
            , JOURNAL_DEFAULT_SLICE_MODE ? "true" : "false" // slice
            , -SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION
            , SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY
            , JOURNAL_DEFAULT_DIRECTION == FACETS_ANCHOR_DIRECTION_BACKWARD ? "backward" : "forward"
    );

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

const char *errno_map[] = {
        [1] = "1 (EPERM)",          // "Operation not permitted",
        [2] = "2 (ENOENT)",         // "No such file or directory",
        [3] = "3 (ESRCH)",          // "No such process",
        [4] = "4 (EINTR)",          // "Interrupted system call",
        [5] = "5 (EIO)",            // "Input/output error",
        [6] = "6 (ENXIO)",          // "No such device or address",
        [7] = "7 (E2BIG)",          // "Argument list too long",
        [8] = "8 (ENOEXEC)",        // "Exec format error",
        [9] = "9 (EBADF)",          // "Bad file descriptor",
        [10] = "10 (ECHILD)",        // "No child processes",
        [11] = "11 (EAGAIN)",        // "Resource temporarily unavailable",
        [12] = "12 (ENOMEM)",        // "Cannot allocate memory",
        [13] = "13 (EACCES)",        // "Permission denied",
        [14] = "14 (EFAULT)",        // "Bad address",
        [15] = "15 (ENOTBLK)",       // "Block device required",
        [16] = "16 (EBUSY)",         // "Device or resource busy",
        [17] = "17 (EEXIST)",        // "File exists",
        [18] = "18 (EXDEV)",         // "Invalid cross-device link",
        [19] = "19 (ENODEV)",        // "No such device",
        [20] = "20 (ENOTDIR)",       // "Not a directory",
        [21] = "21 (EISDIR)",        // "Is a directory",
        [22] = "22 (EINVAL)",        // "Invalid argument",
        [23] = "23 (ENFILE)",        // "Too many open files in system",
        [24] = "24 (EMFILE)",        // "Too many open files",
        [25] = "25 (ENOTTY)",        // "Inappropriate ioctl for device",
        [26] = "26 (ETXTBSY)",       // "Text file busy",
        [27] = "27 (EFBIG)",         // "File too large",
        [28] = "28 (ENOSPC)",        // "No space left on device",
        [29] = "29 (ESPIPE)",        // "Illegal seek",
        [30] = "30 (EROFS)",         // "Read-only file system",
        [31] = "31 (EMLINK)",        // "Too many links",
        [32] = "32 (EPIPE)",         // "Broken pipe",
        [33] = "33 (EDOM)",          // "Numerical argument out of domain",
        [34] = "34 (ERANGE)",        // "Numerical result out of range",
        [35] = "35 (EDEADLK)",       // "Resource deadlock avoided",
        [36] = "36 (ENAMETOOLONG)",  // "File name too long",
        [37] = "37 (ENOLCK)",        // "No locks available",
        [38] = "38 (ENOSYS)",        // "Function not implemented",
        [39] = "39 (ENOTEMPTY)",     // "Directory not empty",
        [40] = "40 (ELOOP)",         // "Too many levels of symbolic links",
        [42] = "42 (ENOMSG)",        // "No message of desired type",
        [43] = "43 (EIDRM)",         // "Identifier removed",
        [44] = "44 (ECHRNG)",        // "Channel number out of range",
        [45] = "45 (EL2NSYNC)",      // "Level 2 not synchronized",
        [46] = "46 (EL3HLT)",        // "Level 3 halted",
        [47] = "47 (EL3RST)",        // "Level 3 reset",
        [48] = "48 (ELNRNG)",        // "Link number out of range",
        [49] = "49 (EUNATCH)",       // "Protocol driver not attached",
        [50] = "50 (ENOCSI)",        // "No CSI structure available",
        [51] = "51 (EL2HLT)",        // "Level 2 halted",
        [52] = "52 (EBADE)",         // "Invalid exchange",
        [53] = "53 (EBADR)",         // "Invalid request descriptor",
        [54] = "54 (EXFULL)",        // "Exchange full",
        [55] = "55 (ENOANO)",        // "No anode",
        [56] = "56 (EBADRQC)",       // "Invalid request code",
        [57] = "57 (EBADSLT)",       // "Invalid slot",
        [59] = "59 (EBFONT)",        // "Bad font file format",
        [60] = "60 (ENOSTR)",        // "Device not a stream",
        [61] = "61 (ENODATA)",       // "No data available",
        [62] = "62 (ETIME)",         // "Timer expired",
        [63] = "63 (ENOSR)",         // "Out of streams resources",
        [64] = "64 (ENONET)",        // "Machine is not on the network",
        [65] = "65 (ENOPKG)",        // "Package not installed",
        [66] = "66 (EREMOTE)",       // "Object is remote",
        [67] = "67 (ENOLINK)",       // "Link has been severed",
        [68] = "68 (EADV)",          // "Advertise error",
        [69] = "69 (ESRMNT)",        // "Srmount error",
        [70] = "70 (ECOMM)",         // "Communication error on send",
        [71] = "71 (EPROTO)",        // "Protocol error",
        [72] = "72 (EMULTIHOP)",     // "Multihop attempted",
        [73] = "73 (EDOTDOT)",       // "RFS specific error",
        [74] = "74 (EBADMSG)",       // "Bad message",
        [75] = "75 (EOVERFLOW)",     // "Value too large for defined data type",
        [76] = "76 (ENOTUNIQ)",      // "Name not unique on network",
        [77] = "77 (EBADFD)",        // "File descriptor in bad state",
        [78] = "78 (EREMCHG)",       // "Remote address changed",
        [79] = "79 (ELIBACC)",       // "Can not access a needed shared library",
        [80] = "80 (ELIBBAD)",       // "Accessing a corrupted shared library",
        [81] = "81 (ELIBSCN)",       // ".lib section in a.out corrupted",
        [82] = "82 (ELIBMAX)",       // "Attempting to link in too many shared libraries",
        [83] = "83 (ELIBEXEC)",      // "Cannot exec a shared library directly",
        [84] = "84 (EILSEQ)",        // "Invalid or incomplete multibyte or wide character",
        [85] = "85 (ERESTART)",      // "Interrupted system call should be restarted",
        [86] = "86 (ESTRPIPE)",      // "Streams pipe error",
        [87] = "87 (EUSERS)",        // "Too many users",
        [88] = "88 (ENOTSOCK)",      // "Socket operation on non-socket",
        [89] = "89 (EDESTADDRREQ)",  // "Destination address required",
        [90] = "90 (EMSGSIZE)",      // "Message too long",
        [91] = "91 (EPROTOTYPE)",    // "Protocol wrong type for socket",
        [92] = "92 (ENOPROTOOPT)",   // "Protocol not available",
        [93] = "93 (EPROTONOSUPPORT)",   // "Protocol not supported",
        [94] = "94 (ESOCKTNOSUPPORT)",   // "Socket type not supported",
        [95] = "95 (ENOTSUP)",       // "Operation not supported",
        [96] = "96 (EPFNOSUPPORT)",  // "Protocol family not supported",
        [97] = "97 (EAFNOSUPPORT)",  // "Address family not supported by protocol",
        [98] = "98 (EADDRINUSE)",    // "Address already in use",
        [99] = "99 (EADDRNOTAVAIL)", // "Cannot assign requested address",
        [100] = "100 (ENETDOWN)",     // "Network is down",
        [101] = "101 (ENETUNREACH)",  // "Network is unreachable",
        [102] = "102 (ENETRESET)",    // "Network dropped connection on reset",
        [103] = "103 (ECONNABORTED)", // "Software caused connection abort",
        [104] = "104 (ECONNRESET)",   // "Connection reset by peer",
        [105] = "105 (ENOBUFS)",      // "No buffer space available",
        [106] = "106 (EISCONN)",      // "Transport endpoint is already connected",
        [107] = "107 (ENOTCONN)",     // "Transport endpoint is not connected",
        [108] = "108 (ESHUTDOWN)",    // "Cannot send after transport endpoint shutdown",
        [109] = "109 (ETOOMANYREFS)", // "Too many references: cannot splice",
        [110] = "110 (ETIMEDOUT)",    // "Connection timed out",
        [111] = "111 (ECONNREFUSED)", // "Connection refused",
        [112] = "112 (EHOSTDOWN)",    // "Host is down",
        [113] = "113 (EHOSTUNREACH)", // "No route to host",
        [114] = "114 (EALREADY)",     // "Operation already in progress",
        [115] = "115 (EINPROGRESS)",  // "Operation now in progress",
        [116] = "116 (ESTALE)",       // "Stale file handle",
        [117] = "117 (EUCLEAN)",      // "Structure needs cleaning",
        [118] = "118 (ENOTNAM)",      // "Not a XENIX named type file",
        [119] = "119 (ENAVAIL)",      // "No XENIX semaphores available",
        [120] = "120 (EISNAM)",       // "Is a named type file",
        [121] = "121 (EREMOTEIO)",    // "Remote I/O error",
        [122] = "122 (EDQUOT)",       // "Disk quota exceeded",
        [123] = "123 (ENOMEDIUM)",    // "No medium found",
        [124] = "124 (EMEDIUMTYPE)",  // "Wrong medium type",
        [125] = "125 (ECANCELED)",    // "Operation canceled",
        [126] = "126 (ENOKEY)",       // "Required key not available",
        [127] = "127 (EKEYEXPIRED)",  // "Key has expired",
        [128] = "128 (EKEYREVOKED)",  // "Key has been revoked",
        [129] = "129 (EKEYREJECTED)", // "Key was rejected by service",
        [130] = "130 (EOWNERDEAD)",   // "Owner died",
        [131] = "131 (ENOTRECOVERABLE)",  // "State not recoverable",
        [132] = "132 (ERFKILL)",      // "Operation not possible due to RF-kill",
        [133] = "133 (EHWPOISON)",    // "Memory page has hardware error",
};

static const char *syslog_facility_to_name(int facility) {
    switch (facility) {
        case LOG_FAC(LOG_KERN): return "kern";
        case LOG_FAC(LOG_USER): return "user";
        case LOG_FAC(LOG_MAIL): return "mail";
        case LOG_FAC(LOG_DAEMON): return "daemon";
        case LOG_FAC(LOG_AUTH): return "auth";
        case LOG_FAC(LOG_SYSLOG): return "syslog";
        case LOG_FAC(LOG_LPR): return "lpr";
        case LOG_FAC(LOG_NEWS): return "news";
        case LOG_FAC(LOG_UUCP): return "uucp";
        case LOG_FAC(LOG_CRON): return "cron";
        case LOG_FAC(LOG_AUTHPRIV): return "authpriv";
        case LOG_FAC(LOG_FTP): return "ftp";
        case LOG_FAC(LOG_LOCAL0): return "local0";
        case LOG_FAC(LOG_LOCAL1): return "local1";
        case LOG_FAC(LOG_LOCAL2): return "local2";
        case LOG_FAC(LOG_LOCAL3): return "local3";
        case LOG_FAC(LOG_LOCAL4): return "local4";
        case LOG_FAC(LOG_LOCAL5): return "local5";
        case LOG_FAC(LOG_LOCAL6): return "local6";
        case LOG_FAC(LOG_LOCAL7): return "local7";
        default: return NULL;
    }
}

static const char *syslog_priority_to_name(int priority) {
    switch (priority) {
        case LOG_ALERT: return "alert";
        case LOG_CRIT: return "critical";
        case LOG_DEBUG: return "debug";
        case LOG_EMERG: return "panic";
        case LOG_ERR: return "error";
        case LOG_INFO: return "info";
        case LOG_NOTICE: return "notice";
        case LOG_WARNING: return "warning";
        default: return NULL;
    }
}

static FACET_ROW_SEVERITY syslog_priority_to_facet_severity(FACETS *facets __maybe_unused, FACET_ROW *row, void *data __maybe_unused) {
    // same to
    // https://github.com/systemd/systemd/blob/aab9e4b2b86905a15944a1ac81e471b5b7075932/src/basic/terminal-util.c#L1501
    // function get_log_colors()

    FACET_ROW_KEY_VALUE *priority_rkv = dictionary_get(row->dict, "PRIORITY");
    if(!priority_rkv || priority_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int priority = str2i(buffer_tostring(priority_rkv->wb));

    if(priority <= LOG_ERR)
        return FACET_ROW_SEVERITY_CRITICAL;

    else if (priority <= LOG_WARNING)
        return FACET_ROW_SEVERITY_WARNING;

    else if(priority <= LOG_NOTICE)
        return FACET_ROW_SEVERITY_NOTICE;

    else if(priority >= LOG_DEBUG)
        return FACET_ROW_SEVERITY_DEBUG;

    return FACET_ROW_SEVERITY_NORMAL;
}

static char *uid_to_username(uid_t uid, char *buffer, size_t buffer_size) {
    static __thread char tmp[1024 + 1];
    struct passwd pw, *result = NULL;

    if (getpwuid_r(uid, &pw, tmp, sizeof(tmp), &result) != 0 || !result || !pw.pw_name || !(*pw.pw_name))
        snprintfz(buffer, buffer_size - 1, "%u", uid);
    else
        snprintfz(buffer, buffer_size - 1, "%u (%s)", uid, pw.pw_name);

    return buffer;
}

static char *gid_to_groupname(gid_t gid, char* buffer, size_t buffer_size) {
    static __thread char tmp[1024];
    struct group grp, *result = NULL;

    if (getgrgid_r(gid, &grp, tmp, sizeof(tmp), &result) != 0 || !result || !grp.gr_name || !(*grp.gr_name))
        snprintfz(buffer, buffer_size - 1, "%u", gid);
    else
        snprintfz(buffer, buffer_size - 1, "%u (%s)", gid, grp.gr_name);

    return buffer;
}

static void netdata_systemd_journal_transform_syslog_facility(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        int facility = str2i(buffer_tostring(wb));
        const char *name = syslog_facility_to_name(facility);
        if (name) {
            buffer_flush(wb);
            buffer_strcat(wb, name);
        }
    }
}

static void netdata_systemd_journal_transform_priority(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        int priority = str2i(buffer_tostring(wb));
        const char *name = syslog_priority_to_name(priority);
        if (name) {
            buffer_flush(wb);
            buffer_strcat(wb, name);
        }
    }
}

static void netdata_systemd_journal_transform_errno(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        unsigned err_no = str2u(buffer_tostring(wb));
        if(err_no > 0 && err_no < sizeof(errno_map) / sizeof(*errno_map)) {
            const char *name = errno_map[err_no];
            if(name) {
                buffer_flush(wb);
                buffer_strcat(wb, name);
            }
        }
    }
}

// ----------------------------------------------------------------------------
// UID and GID transformation

#define UID_GID_HASHTABLE_SIZE 10000

struct word_t2str_hashtable_entry {
    struct word_t2str_hashtable_entry *next;
    Word_t hash;
    size_t len;
    char str[];
};

struct word_t2str_hashtable {
    SPINLOCK spinlock;
    size_t size;
    struct word_t2str_hashtable_entry *hashtable[UID_GID_HASHTABLE_SIZE];
};

struct word_t2str_hashtable uid_hashtable = {
        .size = UID_GID_HASHTABLE_SIZE,
};

struct word_t2str_hashtable gid_hashtable = {
        .size = UID_GID_HASHTABLE_SIZE,
};

struct word_t2str_hashtable_entry **word_t2str_hashtable_slot(struct word_t2str_hashtable *ht, Word_t hash) {
    size_t slot = hash % ht->size;
    struct word_t2str_hashtable_entry **e = &ht->hashtable[slot];

    while(*e && (*e)->hash != hash)
        e = &((*e)->next);

    return e;
}

const char *uid_to_username_cached(uid_t uid, size_t *length) {
    spinlock_lock(&uid_hashtable.spinlock);

    struct word_t2str_hashtable_entry **e = word_t2str_hashtable_slot(&uid_hashtable, uid);
    if(!(*e)) {
        static __thread char buf[1024];
        const char *name = uid_to_username(uid, buf, sizeof(buf));
        size_t size = strlen(name) + 1;

        *e = callocz(1, sizeof(struct word_t2str_hashtable_entry) + size);
        (*e)->len = size - 1;
        (*e)->hash = uid;
        memcpy((*e)->str, name, size);
    }

    spinlock_unlock(&uid_hashtable.spinlock);

    *length = (*e)->len;
    return (*e)->str;
}

const char *gid_to_groupname_cached(gid_t gid, size_t *length) {
    spinlock_lock(&gid_hashtable.spinlock);

    struct word_t2str_hashtable_entry **e = word_t2str_hashtable_slot(&gid_hashtable, gid);
    if(!(*e)) {
        static __thread char buf[1024];
        const char *name = gid_to_groupname(gid, buf, sizeof(buf));
        size_t size = strlen(name) + 1;

        *e = callocz(1, sizeof(struct word_t2str_hashtable_entry) + size);
        (*e)->len = size - 1;
        (*e)->hash = gid;
        memcpy((*e)->str, name, size);
    }

    spinlock_unlock(&gid_hashtable.spinlock);

    *length = (*e)->len;
    return (*e)->str;
}

DICTIONARY *boot_ids_to_first_ut = NULL;

static void netdata_systemd_journal_transform_boot_id(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *boot_id = buffer_tostring(wb);
    if(*boot_id && isxdigit(*boot_id)) {
        usec_t ut = UINT64_MAX;
        usec_t *p_ut = dictionary_get(boot_ids_to_first_ut, boot_id);
        if(!p_ut) {
            struct journal_file *jf;
            dfe_start_read(journal_files_registry, jf) {
                const char *files[2] = {
                        [0] = jf_dfe.name,
                        [1] = NULL,
                };

                sd_journal *j = NULL;
                if(sd_journal_open_files(&j, files, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
                    internal_error(true, "JOURNAL: cannot open file '%s' to get boot_id", jf_dfe.name);
                    continue;
                }

                char m[100];
                size_t len = snprintfz(m, sizeof(m), "_BOOT_ID=%s", boot_id);

                if(sd_journal_add_match(j, m, len) < 0) {
                    internal_error(true, "JOURNAL: cannot add match '%s' to file '%s'", m, jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(sd_journal_seek_head(j) < 0) {
                    internal_error(true, "JOURNAL: cannot seek head to file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(sd_journal_next(j) < 0) {
                    internal_error(true, "JOURNAL: cannot get next of file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                usec_t t_ut = 0;
                if(sd_journal_get_realtime_usec(j, &t_ut) < 0 || !t_ut) {
                    internal_error(true, "JOURNAL: cannot get realtime_usec of file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(t_ut < ut)
                    ut = t_ut;

                sd_journal_close(j);
            }
            dfe_done(jf);

            dictionary_set(boot_ids_to_first_ut, boot_id, &ut, sizeof(ut));
        }
        else
            ut = *p_ut;

        if(ut != UINT64_MAX) {
            time_t timestamp_sec = (time_t)(ut / USEC_PER_SEC);
            struct tm tm;
            char buffer[30];

            gmtime_r(&timestamp_sec, &tm);
            strftime(buffer, sizeof(buffer), "%Y-%m-%d %H:%M:%S", &tm);

            switch(scope) {
                default:
                case FACETS_TRANSFORM_DATA:
                case FACETS_TRANSFORM_VALUE:
                    buffer_sprintf(wb, " (%s UTC)  ", buffer);
                    break;

                case FACETS_TRANSFORM_FACET:
                case FACETS_TRANSFORM_FACET_SORT:
                case FACETS_TRANSFORM_HISTOGRAM:
                    buffer_flush(wb);
                    buffer_sprintf(wb, "%s UTC", buffer);
                    break;
            }
        }
    }
}

static void netdata_systemd_journal_transform_uid(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uid_t uid = str2i(buffer_tostring(wb));
        size_t len;
        const char *name = uid_to_username_cached(uid, &len);
        buffer_contents_replace(wb, name, len);
    }
}

static void netdata_systemd_journal_transform_gid(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        gid_t gid = str2i(buffer_tostring(wb));
        size_t len;
        const char *name = gid_to_groupname_cached(gid, &len);
        buffer_contents_replace(wb, name, len);
    }
}

const char *linux_capabilities[] = {
        [CAP_CHOWN] = "CHOWN",
        [CAP_DAC_OVERRIDE] = "DAC_OVERRIDE",
        [CAP_DAC_READ_SEARCH] = "DAC_READ_SEARCH",
        [CAP_FOWNER] = "FOWNER",
        [CAP_FSETID] = "FSETID",
        [CAP_KILL] = "KILL",
        [CAP_SETGID] = "SETGID",
        [CAP_SETUID] = "SETUID",
        [CAP_SETPCAP] = "SETPCAP",
        [CAP_LINUX_IMMUTABLE] = "LINUX_IMMUTABLE",
        [CAP_NET_BIND_SERVICE] = "NET_BIND_SERVICE",
        [CAP_NET_BROADCAST] = "NET_BROADCAST",
        [CAP_NET_ADMIN] = "NET_ADMIN",
        [CAP_NET_RAW] = "NET_RAW",
        [CAP_IPC_LOCK] = "IPC_LOCK",
        [CAP_IPC_OWNER] = "IPC_OWNER",
        [CAP_SYS_MODULE] = "SYS_MODULE",
        [CAP_SYS_RAWIO] = "SYS_RAWIO",
        [CAP_SYS_CHROOT] = "SYS_CHROOT",
        [CAP_SYS_PTRACE] = "SYS_PTRACE",
        [CAP_SYS_PACCT] = "SYS_PACCT",
        [CAP_SYS_ADMIN] = "SYS_ADMIN",
        [CAP_SYS_BOOT] = "SYS_BOOT",
        [CAP_SYS_NICE] = "SYS_NICE",
        [CAP_SYS_RESOURCE] = "SYS_RESOURCE",
        [CAP_SYS_TIME] = "SYS_TIME",
        [CAP_SYS_TTY_CONFIG] = "SYS_TTY_CONFIG",
        [CAP_MKNOD] = "MKNOD",
        [CAP_LEASE] = "LEASE",
        [CAP_AUDIT_WRITE] = "AUDIT_WRITE",
        [CAP_AUDIT_CONTROL] = "AUDIT_CONTROL",
        [CAP_SETFCAP] = "SETFCAP",
        [CAP_MAC_OVERRIDE] = "MAC_OVERRIDE",
        [CAP_MAC_ADMIN] = "MAC_ADMIN",
        [CAP_SYSLOG] = "SYSLOG",
        [CAP_WAKE_ALARM] = "WAKE_ALARM",
        [CAP_BLOCK_SUSPEND] = "BLOCK_SUSPEND",
        [37 /*CAP_AUDIT_READ*/] = "AUDIT_READ",
        [38 /*CAP_PERFMON*/] = "PERFMON",
        [39 /*CAP_BPF*/] = "BPF",
        [40 /* CAP_CHECKPOINT_RESTORE */] = "CHECKPOINT_RESTORE",
};

static void netdata_systemd_journal_transform_cap_effective(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uint64_t cap = strtoul(buffer_tostring(wb), NULL, 16);
        if(cap) {
            buffer_fast_strcat(wb, " (", 2);
            for (size_t i = 0, added = 0; i < sizeof(linux_capabilities) / sizeof(linux_capabilities[0]); i++) {
                if (linux_capabilities[i] && (cap & (1ULL << i))) {

                    if (added)
                        buffer_fast_strcat(wb, " | ", 3);

                    buffer_strcat(wb, linux_capabilities[i]);
                    added++;
                }
            }
            buffer_fast_strcat(wb, ")", 1);
        }
    }
}

static void netdata_systemd_journal_transform_timestamp_usec(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uint64_t ut = str2ull(buffer_tostring(wb), NULL);
        if(ut) {
            time_t timestamp_sec = ut / USEC_PER_SEC;
            struct tm tm;
            char buffer[30];

            gmtime_r(&timestamp_sec, &tm);
            strftime(buffer, sizeof(buffer), "%Y-%m-%d %H:%M:%S", &tm);
            buffer_sprintf(wb, " (%s.%06llu UTC)", buffer, ut % USEC_PER_SEC);
        }
    }
}

// ----------------------------------------------------------------------------

static void netdata_systemd_journal_dynamic_row_id(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *pid_rkv = dictionary_get(row->dict, "_PID");
    const char *pid = pid_rkv ? buffer_tostring(pid_rkv->wb) : FACET_VALUE_UNSET;

    const char *identifier = NULL;
    FACET_ROW_KEY_VALUE *container_name_rkv = dictionary_get(row->dict, "CONTAINER_NAME");
    if(container_name_rkv && !container_name_rkv->empty)
        identifier = buffer_tostring(container_name_rkv->wb);

    if(!identifier) {
        FACET_ROW_KEY_VALUE *syslog_identifier_rkv = dictionary_get(row->dict, "SYSLOG_IDENTIFIER");
        if(syslog_identifier_rkv && !syslog_identifier_rkv->empty)
            identifier = buffer_tostring(syslog_identifier_rkv->wb);

        if(!identifier) {
            FACET_ROW_KEY_VALUE *comm_rkv = dictionary_get(row->dict, "_COMM");
            if(comm_rkv && !comm_rkv->empty)
                identifier = buffer_tostring(comm_rkv->wb);
        }
    }

    buffer_flush(rkv->wb);

    if(!identifier || !*identifier)
        buffer_strcat(rkv->wb, FACET_VALUE_UNSET);
    else if(!pid || !*pid)
        buffer_sprintf(rkv->wb, "%s", identifier);
    else
        buffer_sprintf(rkv->wb, "%s[%s]", identifier, pid);

    buffer_json_add_array_item_string(json_array, buffer_tostring(rkv->wb));
}

static void netdata_systemd_journal_rich_message(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row __maybe_unused, void *data __maybe_unused) {
    buffer_json_add_array_item_object(json_array);
    buffer_json_member_add_string(json_array, "value", buffer_tostring(rkv->wb));
    buffer_json_object_close(json_array);
}

DICTIONARY *function_query_status_dict = NULL;

static void function_systemd_journal_progress(BUFFER *wb, const char *transaction, const char *progress_id) {
    if(!progress_id || !(*progress_id)) {
        netdata_mutex_lock(&stdout_mutex);
        pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST, "missing progress id");
        netdata_mutex_unlock(&stdout_mutex);
        return;
    }

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(function_query_status_dict, progress_id);

    if(!item) {
        netdata_mutex_lock(&stdout_mutex);
        pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_NOT_FOUND, "progress id is not found here");
        netdata_mutex_unlock(&stdout_mutex);
        return;
    }

    FUNCTION_QUERY_STATUS *fqs = dictionary_acquired_item_value(item);

    usec_t now_monotonic_ut = now_monotonic_usec();
    if(now_monotonic_ut + 10 * USEC_PER_SEC > fqs->stop_monotonic_ut)
        fqs->stop_monotonic_ut = now_monotonic_ut + 10 * USEC_PER_SEC;

    usec_t duration_ut = now_monotonic_ut - fqs->started_monotonic_ut;

    size_t files_matched = fqs->files_matched;
    size_t file_working = fqs->file_working;
    if(file_working > files_matched)
        files_matched = file_working;

    size_t rows_read = __atomic_load_n(&fqs->rows_read, __ATOMIC_RELAXED);
    size_t bytes_read = __atomic_load_n(&fqs->bytes_read, __ATOMIC_RELAXED);

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_uint64(wb, "running_duration_usec", duration_ut);
    buffer_json_member_add_double(wb, "progress", (double)file_working * 100.0 / (double)files_matched);
    char msg[1024 + 1];
    snprintfz(msg, 1024,
              "Read %zu rows (%0.0f rows/s), "
              "data %0.1f MB (%0.1f MB/s), "
              "file %zu of %zu",
              rows_read, (double)rows_read / (double)duration_ut * (double)USEC_PER_SEC,
              (double)bytes_read / 1024.0 / 1024.0, ((double)bytes_read / (double)duration_ut * (double)USEC_PER_SEC) / 1024.0 / 1024.0,
              file_working, files_matched
              );
    buffer_json_member_add_string(wb, "message", msg);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_realtime_sec() + 1, wb);
    netdata_mutex_unlock(&stdout_mutex);

    dictionary_acquired_item_release(function_query_status_dict, item);
}

static void function_systemd_journal(const char *transaction, char *function, int timeout, bool *cancelled) {
    fstat_thread_calls = 0;
    fstat_thread_cached_responses = 0;
    journal_files_registry_update();

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    usec_t now_monotonic_ut = now_monotonic_usec();
    FUNCTION_QUERY_STATUS tmp_fqs = {
            .cancelled = cancelled,
            .started_monotonic_ut = now_monotonic_ut,
            .stop_monotonic_ut = now_monotonic_ut + (timeout * USEC_PER_SEC),
    };
    FUNCTION_QUERY_STATUS *fqs = NULL;
    const DICTIONARY_ITEM *fqs_item = NULL;

    FACETS *facets = facets_create(50, FACETS_OPTION_ALL_KEYS_FTS,
                                   SYSTEMD_ALWAYS_VISIBLE_KEYS,
                                   SYSTEMD_KEYS_INCLUDED_IN_FACETS,
                                   SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);

    facets_accepted_param(facets, JOURNAL_PARAMETER_INFO);
    facets_accepted_param(facets, JOURNAL_PARAMETER_SOURCE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_AFTER);
    facets_accepted_param(facets, JOURNAL_PARAMETER_BEFORE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_ANCHOR);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DIRECTION);
    facets_accepted_param(facets, JOURNAL_PARAMETER_LAST);
    facets_accepted_param(facets, JOURNAL_PARAMETER_QUERY);
    facets_accepted_param(facets, JOURNAL_PARAMETER_FACETS);
    facets_accepted_param(facets, JOURNAL_PARAMETER_HISTOGRAM);
    facets_accepted_param(facets, JOURNAL_PARAMETER_IF_MODIFIED_SINCE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DATA_ONLY);
    facets_accepted_param(facets, JOURNAL_PARAMETER_ID);
    facets_accepted_param(facets, JOURNAL_PARAMETER_PROGRESS);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DELTA);
    facets_accepted_param(facets, JOURNAL_PARAMETER_TAIL);

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    facets_accepted_param(facets, JOURNAL_PARAMETER_SLICE);
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    // register the fields in the order you want them on the dashboard

    facets_register_row_severity(facets, syslog_priority_to_facet_severity, NULL);

    facets_register_key_name(facets, "_HOSTNAME",
                             FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_VISIBLE);

    facets_register_dynamic_key_name(facets, JOURNAL_KEY_ND_JOURNAL_PROCESS,
                                     FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_VISIBLE,
                                     netdata_systemd_journal_dynamic_row_id, NULL);

    facets_register_key_name(facets, "MESSAGE",
                                     FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT |
                                     FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

//    facets_register_dynamic_key_name(facets, "MESSAGE",
//                             FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_RICH_TEXT |
//                             FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS,
//                             netdata_systemd_journal_rich_message, NULL);

    facets_register_key_name_transformation(facets, "PRIORITY",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_priority, NULL);

    facets_register_key_name_transformation(facets, "SYSLOG_FACILITY",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_syslog_facility, NULL);

    facets_register_key_name_transformation(facets, "ERRNO",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_errno, NULL);

    facets_register_key_name(facets, JOURNAL_KEY_ND_JOURNAL_FILE,
                             FACET_KEY_OPTION_NEVER_FACET);

    facets_register_key_name(facets, "SYSLOG_IDENTIFIER",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name(facets, "UNIT",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name(facets, "USER_UNIT",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name_transformation(facets, "_BOOT_ID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_boot_id, NULL);

    facets_register_key_name_transformation(facets, "_SYSTEMD_OWNER_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_SYSTEMD_OWNER_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_GID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_GID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(facets, "_CAP_EFFECTIVE",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_cap_effective, NULL);

    facets_register_key_name_transformation(facets, "_AUDIT_LOGINUID",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_AUDIT_LOGINUID",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_SOURCE_REALTIME_TIMESTAMP",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_timestamp_usec, NULL);

    // ------------------------------------------------------------------------
    // parse the parameters

    bool info = false, data_only = false, progress = false, slice = JOURNAL_DEFAULT_SLICE_MODE, delta = false, tail = false;
    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    usec_t if_modified_since = 0;
    size_t last = 0;
    FACETS_ANCHOR_DIRECTION direction = JOURNAL_DEFAULT_DIRECTION;
    const char *query = NULL;
    const char *chart = NULL;
    SIMPLE_PATTERN *sources = NULL;
    const char *progress_id = NULL;
    SD_JOURNAL_FILE_SOURCE_TYPE source_type = SDJF_ALL;
    size_t filters = 0;

    buffer_json_member_add_object(wb, "_request");

    char *words[SYSTEMD_JOURNAL_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_JOURNAL_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_JOURNAL_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, JOURNAL_PARAMETER_HELP) == 0) {
            netdata_systemd_journal_function_help(transaction);
            goto cleanup;
        }
        else if(strcmp(keyword, JOURNAL_PARAMETER_INFO) == 0) {
            info = true;
        }
        else if(strcmp(keyword, JOURNAL_PARAMETER_PROGRESS) == 0) {
            progress = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DELTA ":", sizeof(JOURNAL_PARAMETER_DELTA ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DELTA ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                delta = false;
            else
                delta = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_TAIL ":", sizeof(JOURNAL_PARAMETER_TAIL ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_TAIL ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                tail = false;
            else
                tail = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DATA_ONLY ":", sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                data_only = false;
            else
                data_only = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SLICE ":", sizeof(JOURNAL_PARAMETER_SLICE ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_SLICE ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                slice = false;
            else
                slice = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ID ":", sizeof(JOURNAL_PARAMETER_ID ":") - 1) == 0) {
            char *id = &keyword[sizeof(JOURNAL_PARAMETER_ID ":") - 1];

            if(*id)
                progress_id = id;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SOURCE ":", sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1) == 0) {
            const char *value = &keyword[sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1];

            buffer_json_member_add_array(wb, JOURNAL_PARAMETER_SOURCE);

            BUFFER *sources_list = buffer_create(0, NULL);

            source_type = SDJF_NONE;
            while(value) {
                char *sep = strchr(value, ',');
                if(sep)
                    *sep++ = '\0';

                buffer_json_add_array_item_string(wb, value);

                if(strcmp(value, SDJF_SOURCE_ALL_NAME) == 0) {
                    source_type |= SDJF_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_NAME) == 0) {
                    source_type |= SDJF_LOCAL_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_REMOTES_NAME) == 0) {
                    source_type |= SDJF_REMOTE_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_NAMESPACES_NAME) == 0) {
                    source_type |= SDJF_LOCAL_NAMESPACE;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_SYSTEM_NAME) == 0) {
                    source_type |= SDJF_LOCAL_SYSTEM;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_USERS_NAME) == 0) {
                    source_type |= SDJF_LOCAL_USER;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_OTHER_NAME) == 0) {
                    source_type |= SDJF_LOCAL_OTHER;
                    value = NULL;
                }
                else {
                    // else, match the source, whatever it is
                    if(buffer_strlen(sources_list))
                        buffer_strcat(sources_list, ",");

                    buffer_strcat(sources_list, value);
                }

                value = sep;
            }

            if(buffer_strlen(sources_list)) {
                simple_pattern_free(sources);
                sources = simple_pattern_create(buffer_tostring(sources_list), ",", SIMPLE_PATTERN_EXACT, false);
            }

            buffer_free(sources_list);

            buffer_json_array_close(wb); // source
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_AFTER ":", sizeof(JOURNAL_PARAMETER_AFTER ":") - 1) == 0) {
            after_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE ":", sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1) == 0) {
            before_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":", sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1) == 0) {
            if_modified_since = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ANCHOR ":", sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1) == 0) {
            anchor = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DIRECTION ":", sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1) == 0) {
            direction = strcasecmp(&keyword[sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1], "forward") == 0 ? FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_LAST ":", sizeof(JOURNAL_PARAMETER_LAST ":") - 1) == 0) {
            last = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_LAST ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_QUERY ":", sizeof(JOURNAL_PARAMETER_QUERY ":") - 1) == 0) {
            query= &keyword[sizeof(JOURNAL_PARAMETER_QUERY ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_HISTOGRAM ":", sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1) == 0) {
            chart = &keyword[sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_FACETS ":", sizeof(JOURNAL_PARAMETER_FACETS ":") - 1) == 0) {
            char *value = &keyword[sizeof(JOURNAL_PARAMETER_FACETS ":") - 1];
            if(*value) {
                buffer_json_member_add_array(wb, JOURNAL_PARAMETER_FACETS);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // JOURNAL_PARAMETER_FACETS
            }
        }
        else {
            char *value = strchr(keyword, ':');
            if(value) {
                *value++ = '\0';

                buffer_json_member_add_array(wb, keyword);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id_filter(facets, keyword, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);
                    filters++;

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    // ------------------------------------------------------------------------
    // put this request into the progress db

    if(progress_id && *progress_id) {
        fqs_item = dictionary_set_and_acquire_item(function_query_status_dict, progress_id, &tmp_fqs, sizeof(tmp_fqs));
        fqs = dictionary_acquired_item_value(fqs_item);
    }
    else {
        // no progress id given, proceed without registering our progress in the dictionary
        fqs = &tmp_fqs;
        fqs_item = NULL;
    }

    // ------------------------------------------------------------------------
    // validate parameters

    time_t now_s = now_realtime_sec();
    time_t expires = now_s + 1;

    if(!after_s && !before_s) {
        before_s = now_s;
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&after_s, &before_s, now_s);

    if(after_s > before_s) {
        time_t tmp = after_s;
        after_s = before_s;
        before_s = tmp;
    }

    if(after_s == before_s)
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;

    if(!last)
        last = SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY;


    // ------------------------------------------------------------------------
    // set query time-frame, anchors and direction

    fqs->after_ut = after_s * USEC_PER_SEC;
    fqs->before_ut = (before_s * USEC_PER_SEC) + USEC_PER_SEC - 1;
    fqs->if_modified_since = if_modified_since;
    fqs->data_only = data_only;
    fqs->delta = (fqs->data_only) ? delta : false;
    fqs->tail = (fqs->data_only && fqs->if_modified_since) ? tail : false;
    fqs->sources = sources;
    fqs->source_type = source_type;
    fqs->entries = last;
    fqs->last_modified = 0;
    fqs->filters = filters;
    fqs->query = (query && *query) ? query : NULL;
    fqs->histogram = (chart && *chart) ? chart : NULL;
    fqs->direction = direction;
    fqs->anchor.start_ut = anchor;
    fqs->anchor.stop_ut = 0;

    if(fqs->anchor.start_ut && fqs->tail) {
        // a tail request
        // we need the top X entries from BEFORE
        // but, we need to calculate the facets and the
        // histogram up to the anchor
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = anchor;
    }

    if(anchor && anchor < fqs->after_ut) {
        log_fqs(fqs, "received anchor is too small for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }
    else if(anchor > fqs->before_ut) {
        log_fqs(fqs, "received anchor is too big for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }

    facets_set_anchor(facets, fqs->anchor.start_ut, fqs->anchor.stop_ut, fqs->direction);

    facets_set_additional_options(facets,
                                  ((fqs->data_only) ? FACETS_OPTION_DATA_ONLY : 0) |
                                  ((fqs->delta) ? FACETS_OPTION_SHOW_DELTAS : 0));

    // ------------------------------------------------------------------------
    // set the rest of the query parameters


    facets_set_items(facets, fqs->entries);
    facets_set_query(facets, fqs->query);

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    fqs->slice = slice;
    if(slice)
        facets_enable_slice_mode(facets);
#else
    fqs->slice = false;
#endif

    if(fqs->histogram)
        facets_set_timeframe_and_histogram_by_id(facets, fqs->histogram, fqs->after_ut, fqs->before_ut);
    else
        facets_set_timeframe_and_histogram_by_name(facets, "PRIORITY", fqs->after_ut, fqs->before_ut);


    // ------------------------------------------------------------------------
    // complete the request object

    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_INFO, false);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_SLICE, fqs->slice);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DATA_ONLY, fqs->data_only);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_PROGRESS, false);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DELTA, fqs->delta);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_TAIL, fqs->tail);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_ID, progress_id);
    buffer_json_member_add_uint64(wb, "source_type", fqs->source_type);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_AFTER, fqs->after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_BEFORE, fqs->before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, "if_modified_since", fqs->if_modified_since);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_ANCHOR, anchor);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_DIRECTION, fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_LAST, fqs->entries);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_QUERY, fqs->query);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_HISTOGRAM, fqs->histogram);
    buffer_json_object_close(wb); // request

    buffer_json_journal_versions(wb);

    // ------------------------------------------------------------------------
    // run the request

    int response;

    if(info) {
        facets_accepted_parameters_to_json_array(facets, wb, false);
        buffer_json_member_add_array(wb, "required_params");
        {
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "source");
                buffer_json_member_add_string(wb, "name", "source");
                buffer_json_member_add_string(wb, "help", "Select the SystemD Journal source to query");
                buffer_json_member_add_string(wb, "type", "multiselect");
                buffer_json_member_add_array(wb, "options");
                {
                    available_journal_file_sources_to_json_array(wb);
                }
                buffer_json_array_close(wb); // options array
            }
            buffer_json_object_close(wb); // required params object
        }
        buffer_json_array_close(wb); // required_params array

        facets_table_config(wb);

        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "table");
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
        buffer_json_finalize(wb);
        response = HTTP_RESP_OK;
        goto output;
    }

    if(progress) {
        function_systemd_journal_progress(wb, transaction, progress_id);
        goto cleanup;
    }

    response = netdata_systemd_journal_query(wb, facets, fqs);

    // ------------------------------------------------------------------------
    // handle error response

    if(response != HTTP_RESP_OK) {
        netdata_mutex_lock(&stdout_mutex);
        pluginsd_function_json_error_to_stdout(transaction, response, "failed");
        netdata_mutex_unlock(&stdout_mutex);
        goto cleanup;
    }

output:
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, response, "application/json", expires, wb);
    netdata_mutex_unlock(&stdout_mutex);

cleanup:
    simple_pattern_free(sources);
    facets_destroy(facets);
    buffer_free(wb);

    if(fqs_item) {
        dictionary_del(function_query_status_dict, dictionary_acquired_item_name(fqs_item));
        dictionary_acquired_item_release(function_query_status_dict, fqs_item);
        dictionary_garbage_collect(function_query_status_dict);
    }
}

// ----------------------------------------------------------------------------

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    stderror = stderr;
    clocks_init();

    program_name = "systemd-journal.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    log_set_global_severity_for_external_plugins();

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix() == -1) exit(1);

    // ------------------------------------------------------------------------
    // setup the journal directories

    unsigned d = 0;

    journal_directories[d++].path = strdupz("/var/log/journal");
    journal_directories[d++].path = strdupz("/run/log/journal");

    if(*netdata_configured_host_prefix) {
        char path[PATH_MAX];
        snprintfz(path, sizeof(path), "%s/var/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = strdupz(path);
        snprintfz(path, sizeof(path), "%s/run/log/journal", netdata_configured_host_prefix);
        journal_directories[d++].path = strdupz(path);
    }

    // terminate the list
    journal_directories[d].path = NULL;

    // ------------------------------------------------------------------------

    function_query_status_dict = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(FUNCTION_QUERY_STATUS));

    // ------------------------------------------------------------------------
    // initialize the used hashes files registry

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);


    // ------------------------------------------------------------------------
    // initialize the journal files registry

    systemd_journal_session = (now_realtime_usec() / USEC_PER_SEC) * USEC_PER_SEC;

    journal_files_registry = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(struct journal_file));

    dictionary_register_insert_callback(journal_files_registry, files_registry_insert_cb, NULL);
    dictionary_register_delete_callback(journal_files_registry, files_registry_delete_cb, NULL);
    dictionary_register_conflict_callback(journal_files_registry, files_registry_conflict_cb, NULL);

    boot_ids_to_first_ut = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(usec_t));

    journal_files_registry_update();


    // ------------------------------------------------------------------------
    // debug

    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
        bool cancelled = false;
        char buf[] = "systemd-journal after:-16000000 before:0 last:1";
        // char buf[] = "systemd-journal after:1695332964 before:1695937764 direction:backward last:100 slice:true source:all DHKucpqUoe1:PtVoyIuX.MU";
        // char buf[] = "systemd-journal after:1694511062 before:1694514662 anchor:1694514122024403";
        function_systemd_journal("123", buf, 600, &cancelled);
        exit(1);
    }

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(SYSTEMD_JOURNAL_WORKER_THREADS, "SDJ", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg, SYSTEMD_JOURNAL_FUNCTION_NAME, function_systemd_journal,
                                  SYSTEMD_JOURNAL_DEFAULT_TIMEOUT);


    // ------------------------------------------------------------------------

    time_t started_t = now_monotonic_sec();

    size_t iteration = 0;
    usec_t step = 1000 * USEC_PER_MS;
    bool tty = isatty(fileno(stderr)) == 1;

    netdata_mutex_lock(&stdout_mutex);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n",
            SYSTEMD_JOURNAL_FUNCTION_NAME, SYSTEMD_JOURNAL_DEFAULT_TIMEOUT, SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {
        iteration++;

        netdata_mutex_unlock(&stdout_mutex);
        heartbeat_next(&hb, step);
        netdata_mutex_lock(&stdout_mutex);

        if(!tty)
            fprintf(stdout, "\n");

        fflush(stdout);

        time_t now = now_monotonic_sec();
        if(now - started_t > 86400)
            break;
    }

    exit(0);
}
